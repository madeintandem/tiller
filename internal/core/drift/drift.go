// Package drift is Tiller's read-only detector: the schema plants markers
// (artifact-schema.md, "Drift control hooks"), and Run sweeps the
// ones the manifest's drift.checks enables, reporting what moved without
// touching anything. It never mutates a node, writes an event, or refuses a
// caller (cli-surface.md, the drift paragraph: "the command refuses nothing
// and mutates nothing"); the fix is the agent's to draft and the human's to
// approve. Two rules hold across every marker: a candidate exists only where
// a fix exists (a vanished file, an edited spec, an unreviewed answer, never
// "someone touched nearby code"), and a node the sweep could not compare is
// Uncovered with a reason, never folded into a clean report by silence.
//
// This file is the skeleton every later marker plugs into: the checks-list
// validation, the Disabled/Exempted/Coverage footers, and the two markers
// whose comparators already exist elsewhere in the tree, active_basis
// (mutate.BasisDrift) and answered_question (mutate.UnreviewedAssumptions).
// Task 8 added stale_spec and missing_surface, both governed by the vanish
// rule (a candidate only when a recorded file is gone; a modified one is
// context, never a candidate). orphan_eval adds an existence check over
// path-shaped refs, polarity-aware in time
// rather than governed by the vanish rule, since a contract naming a test
// before the test is written is the workflow working, not drift.
// design.go adds the ledger-backed as-is markers, whose comparisons use
// confirmation rather than adoption and count documents separately.
package drift

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/evalref"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// The marker names artifact-schema.md's "Drift control hooks" section
// plants, in the schema's own order (the same order the manifest's example
// drift.checks lists them in). A manifest names a subset of exactly these;
// anything else is Run's one misuse refusal, unknownDriftCheckError.
const (
	MarkerStaleSpec        = "stale_spec"
	MarkerActiveBasis      = "active_basis"
	MarkerAnsweredQuestion = "answered_question"
	MarkerMissingSurface   = "missing_surface"
	MarkerOrphanEval       = "orphan_eval"
	MarkerStaleDesign      = "stale_design"
	MarkerShiftedDesign    = "shifted_design"
)

// knownMarkers is the constants above, ordered, so both the
// checks-list validation and the Disabled footer range over one list rather
// than a set with no order of its own.
var knownMarkers = []string{
	MarkerStaleSpec, MarkerActiveBasis, MarkerAnsweredQuestion, MarkerMissingSurface, MarkerOrphanEval,
	MarkerStaleDesign, MarkerShiftedDesign,
}

// KnownMarkers exports the same ordered list for a caller outside this
// package that needs the identical schema order: the CLI's human and footer
// rendering, so a new marker added
// here is not also silently absent from a duplicated list elsewhere.
var KnownMarkers = knownMarkers

// Candidate is one node a marker's sweep found drifted: a fix exists for it
// (artifact-schema.md, Drift control hooks). Since is the record's commit
// anchor and is nil in two cases, neither of which is evidence the sweep
// could not check the node: a pre-anchor record (a zero
// AcceptedAgainst.Commit), where
// the comparison ran but has nothing to date itself by; and
// answered_question's own second case, where the answer revision is a
// sha256 worktree fingerprint rather than a git_commit (questionSince
// withholds it rather than rendering a fingerprint as if it were a diffable
// commit: a Since can be a fingerprint).
type Candidate struct {
	Marker   string
	Noun     string
	ID       string
	Key      string
	Detail   string             // one human sentence: what fell out of agreement
	Files    []string           // the vanished or drifted files, when file-grained
	Since    *store.RevisionRef // the record's commit anchor; nil when there is none to date by (a pre-anchor record, or answered_question's fingerprint-kind case)
	Until    *store.RevisionRef // optional verified upper history anchor
	Features []DesignFeature    // complete matching done-feature facts for stale_design
	Context  []string           // beside-the-candidate facts, e.g. "14 files changed under app/models"

	// Question is the answered question's id, set by answered_question alone:
	// the structured handle the remedy command composes from
	// (views.ReviewedAssumptionCommand), so no consumer has to parse it back
	// out of Detail's prose (JC's drift-panel ask, 2026-08-06).
	Question string
}

// DesignFeature names tracked work behind a stale as-is description.
type DesignFeature struct {
	ID     string
	Key    string
	Spec   string
	Commit store.RevisionRef
}

// Uncovered is one node a marker's sweep could not compare, with the reason
// it could not (an unhashable path, a done node with no completion record, a
// dangling reference) and a Detail naming the specific thing (two dangling
// question ids on one feature must not read as one indistinguishable row),
// never silently folded into a clean report (artifact-schema.md: "a clean
// answer asserts coverage").
type Uncovered struct {
	Marker string
	Noun   string
	ID     string
	Key    string

	// Reason is scoped PER MARKER, not a single flat, globally-unique
	// vocabulary: two different markers
	// can and do emit the identical token for two different failures, so a
	// consumer aggregating across markers (a report footer, a later Task 10
	// grouping) must key on the PAIR (Marker, Reason), never on Reason alone.
	// Below is which marker emits which, current as of orphan_eval:
	//
	//   - no_completion_record: stale_spec only (a done node with a zero
	//     verified_against).
	//   - basis_unreadable: active_basis only (a pinned spec/eval/task file
	//     that could not be hashed for a reason other than absence).
	//   - unknown_question: answered_question only (assumption_basis names a
	//     question id the ledger no longer carries).
	//   - spec_unreadable: stale_spec only (its own spec-file leg: the
	//     recorded feature.md/epic.md blob could not be re-hashed).
	//   - eval_unreadable: TWO markers, two different meanings. stale_spec
	//     uses it for its own eval-file leg (the recorded feature.eval.md
	//     blob could not be re-hashed, matching its spec leg one line up).
	//     orphan_eval uses it for a feature's WHOLE contract failing to load
	//     or validate at all (a missing key, an unreadable file, malformed
	//     YAML, or a ValidateContract refusal), before any criterion is ever
	//     read. Neither is the other: a stale_spec row names a done node
	//     whose completed contract can no longer be re-verified; an
	//     orphan_eval row names a node whose contract was never readable in
	//     the first place.
	//   - surface_unreadable: TWO markers, the SAME meaning one grain apart.
	//     stale_spec uses it for its own surface leg (a recorded
	//     SurfaceFingerprint's tree hash could not be read); missing_surface
	//     uses it for a plain recorded Surface.Paths entry that could not be
	//     stat'd. Both name "a surface path this marker could not read",
	//     just at a different marker's own grain of "surface".
	//   - ref_unreadable: orphan_eval only (one criterion's own checkable ref,
	//     a test path or a baseline artifact, hit a hard stat failure
	//     distinct from simply being absent).
	Reason string
	Detail string // one human sentence naming the specific thing: the dangling id, the unreadable path
}

// Report is one Run's whole outcome. Candidates and Uncovered are the two
// lists a clean sweep leaves empty. Disabled names every known marker
// drift.checks left enabled-set-complement, so a check absent from the
// manifest is visible rather than silently skipped. Exempted counts the
// nodes a recorded human decision (superseded, archived, cancelled)
// deliberately skips a marker's sweep, per marker.
//
// Coverage states what a marker's sweep actually did with what it looked at
// (buckets like checked, unresolvable, out_of_scope), rather than leaving
// that implied by the two lists alone. A marker's map may count in whatever
// unit suits its own sweep, but it documents that unit where it is built:
// active_basis, answered_question, stale_spec, and missing_surface all count
// NODES (an epic, a feature, or a task), the same unit Compared counts in,
// so a reader of one marker's bucket
// does not have to guess whether it agrees with the total. orphan_eval is
// the one exception, documented at sweepOrphanEval: it counts CRITERIA, the
// finer grain its own existence checks actually operate at, since one
// feature's contract can hold several independently-checkable refs and the
// coverage counts are what keep the path-shaped rule's conservatism honest.
// Compared still counts nodes even there.
//
// Compared is the total count of DISTINCT NODES every enabled marker fully
// compared in this one Run, deduped across markers as well as within one:
// several enabled markers can each complete their own full comparison of the
// IDENTICAL node in one Run (a ready feature, say, swept by active_basis,
// answered_question, missing_surface, AND orphan_eval all at once), and
// Compared counts that node once, not once per marker that reached it
// (markCompared builds the dedup set; Run assigns Compared from its size once
// every enabled marker has finished). Before this dedup, a real 5-node
// scratch tree printed "Compared 9 node(s)": every marker bumped
// independently, inflating the total by one for every marker that touched a
// node (the Compared-counts-marker-visits finding).
//
// A node counts as compared when AT LEAST ONE enabled marker completed its
// own comparison of it, clean or a candidate either way, since the
// comparison itself ran. A node uncovered for every marker that reached it
// counts nowhere. Between those two rules, a node CAN be both compared (one
// marker finished it) and named in Uncovered (a different marker could
// not): the dedup is keyed per node, not per outcome, so one marker's
// success and another marker's failure over the identical node both stand.
// Exit 0's coverage assertion is carried by the Uncovered list being empty,
// not by this number: this field states how much work happened, and
// Uncovered states what, if anything, still needs a human's attention.
type Report struct {
	Candidates     []Candidate
	Uncovered      []Uncovered
	Disabled       []string
	Exempted       map[string]int
	Coverage       map[string]map[string]int
	Compared       int
	ComparedDesign int // distinct ledger documents compared, never work nodes
	comparedDesign map[string]bool

	// comparedNodes is Run's own per-node completion set, keyed "noun:id"
	// (the same compact key classifyBasisDrift's own dedupe already uses one
	// section down). markCompared populates it; Run reads its size into
	// Compared once the whole sweep finishes. Unexported: it is bookkeeping
	// for this package alone, never part of the shape a caller (the CLI's
	// JSON envelope included) reads off a returned Report.
	comparedNodes map[string]bool
}

// unknownDriftCheckError is Run's one misuse-grade refusal: an unknown name
// in drift.checks. The drift package cannot
// import mutate's unexported usageError, so this defines the same shape
// store.EnvError and mutate's usageError both already carry
// (internal/store/manifest.go's EnvError is the pattern this follows): an
// ExitCode for the CLI's classifier (internal/cli/envelope.go's classify,
// which type-switches on store.ExitCoder) and a Reason for its stable
// discriminator (the same file's reasoner interface).
type unknownDriftCheckError struct {
	name string
}

func (e *unknownDriftCheckError) Error() string {
	return fmt.Sprintf("drift.checks names an unknown check %q; known checks are %s",
		e.name, strings.Join(knownMarkers, ", "))
}
func (e *unknownDriftCheckError) ExitCode() int  { return 2 }
func (e *unknownDriftCheckError) Reason() string { return "unknown_drift_check" }

// isKnownMarker reports whether name is one of the schema markers.
func isKnownMarker(name string) bool {
	for _, m := range knownMarkers {
		if m == name {
			return true
		}
	}
	return false
}

// enabledSet validates checks against the known markers and returns the
// enabled subset. An empty or missing drift.checks reads the manifest row
// literally ("tiller drift runs exactly this list"): nothing is enabled, so
// every known marker is later reported in Disabled. That is the same
// reading whether drift: is an empty list or the whole block is absent (an
// old, hand-built manifest), because store.DriftBlock.Checks is nil in both
// cases.
func enabledSet(checks []string) (map[string]bool, error) {
	enabled := make(map[string]bool, len(checks))
	for _, c := range checks {
		if !isKnownMarker(c) {
			return nil, &unknownDriftCheckError{name: c}
		}
		enabled[c] = true
	}
	return enabled, nil
}

// disabledMarkers is the enabled set's complement over knownMarkers, in
// schema order: the report footer that makes disabling a check a deliberate,
// named, in-repo choice rather than a silent skip (artifact-schema.md, the
// "drift" manifest row).
func disabledMarkers(enabled map[string]bool) []string {
	var out []string
	for _, m := range knownMarkers {
		if !enabled[m] {
			out = append(out, m)
		}
	}
	return out
}

// Run validates t's drift.checks, sweeps every marker it enables, and
// aggregates one Report. The returned error is only ever
// unknownDriftCheckError; every other failure a single node's comparison
// hits is Uncovered, never a dead run (cli-surface.md, the drift paragraph).
func Run(t *store.Tree, rev vcs.Revisions) (Report, error) {
	enabled, err := enabledSet(t.Manifest.Drift.Checks)
	if err != nil {
		return Report{}, err
	}

	rep := Report{
		Exempted: map[string]int{},
		Coverage: map[string]map[string]int{},
	}
	rep.Disabled = disabledMarkers(enabled)

	for _, marker := range knownMarkers {
		if !enabled[marker] {
			continue
		}
		switch marker {
		case MarkerStaleSpec:
			sweepStaleSpec(t, rev, &rep)
		case MarkerActiveBasis:
			sweepActiveBasis(t, rev, &rep)
		case MarkerAnsweredQuestion:
			sweepAnsweredQuestion(t, rev, &rep)
		case MarkerMissingSurface:
			sweepMissingSurface(t, &rep)
		case MarkerOrphanEval:
			sweepOrphanEval(t, &rep)
		case MarkerStaleDesign, MarkerShiftedDesign:
			sweepDesign(t, rev, &rep, marker)
		}
	}
	// Compared is read off the dedup set's size here, once, after every
	// enabled marker has finished (markCompared and Report.Compared's own
	// doc comment): the size is the count of DISTINCT nodes any marker
	// compared this Run, never a sum of how many markers each reached one.
	rep.Compared = len(rep.comparedNodes)
	rep.ComparedDesign = len(rep.comparedDesign)
	return rep, nil
}

// ForNode runs every marker drift.checks enables against exactly one node,
// sharing the identical report/finish helpers Run's own sweep calls below
// (never a second implementation of a marker's comparison), so a single
// node's badges can never disagree with what a whole-tree Run would have
// found for it. Task 12 (the node page's Drifted chip) is its one caller: the
// chip needs one node's drift on every request, not the whole tree's.
//
// It returns the raw Candidates and Uncovered rows the enabled markers
// produced for n alone, never Run's own aggregate accounting: Disabled,
// Exempted, Coverage, and Compared all describe a WHOLE sweep, a concept a
// single node has no meaning for. The Report value built here exists only to
// reuse the marker functions' own (t, rev, *Report, ...) signatures and is
// discarded down to its two lists before this returns. The one error, same
// as Run's, is an unknown drift.checks entry; a node this package cannot
// itself compare is Uncovered, never a second error path (this package's own
// doc comment).
func ForNode(t *store.Tree, rev vcs.Revisions, n store.Node) ([]Candidate, []Uncovered, error) {
	enabled, err := enabledSet(t.Manifest.Drift.Checks)
	if err != nil {
		return nil, nil, err
	}

	rep := Report{
		Exempted: map[string]int{},
		Coverage: map[string]map[string]int{},
	}
	for _, marker := range knownMarkers {
		if !enabled[marker] {
			continue
		}
		switch marker {
		case MarkerStaleSpec:
			staleSpecForNode(t, rev, &rep, n)
		case MarkerActiveBasis:
			activeBasisForNode(t, rev, &rep, n)
		case MarkerAnsweredQuestion:
			answeredQuestionForNode(t, &rep, n)
		case MarkerMissingSurface:
			missingSurfaceForNode(t, &rep, n)
		case MarkerOrphanEval:
			orphanEvalForNode(t, &rep, n)
		}
	}
	return rep.Candidates, rep.Uncovered, nil
}

// bumpCoverage records one count in Coverage[marker][bucket], initializing
// either map on first use. Coverage exists so a marker states what it did
// with the nodes it looked at (checked cleanly, or could not be read) rather
// than leaving that implied by the Candidates/Uncovered lists alone.
func bumpCoverage(rep *Report, marker, bucket string) {
	m := rep.Coverage[marker]
	if m == nil {
		m = map[string]int{}
		rep.Coverage[marker] = m
	}
	m[bucket]++
}

// markCompared records that ONE MARKER fully compared the node named by noun
// and id. Every marker-side finish/report function below calls this exactly
// where it used to increment Compared directly; the difference is that
// several markers calling this for the IDENTICAL node in one Run grows the
// set only once, so Run can read Compared's final value from the set's size
// rather than from a counter that summed one bump per marker-visit
// (Report.Compared's own doc comment; the Compared-counts-marker-visits
// finding).
func markCompared(rep *Report, noun, id string) {
	if rep.comparedNodes == nil {
		rep.comparedNodes = map[string]bool{}
	}
	rep.comparedNodes[noun+":"+id] = true
}

// nodeKey reads a node's key, "" for a nil node or a keyless one (a
// question), the same key a Candidate or Uncovered entry carries for the
// reference convention (cli-surface.md).
func nodeKey(n store.Node) string {
	if n == nil {
		return ""
	}
	if k := n.NodeKey(); k != nil {
		return *k
	}
	return ""
}

// --- active_basis ---

// activeBasisFeatureStatuses and activeBasisTaskStatuses are the sweep's own
// status filter, verbatim from the schema's active_basis bullet: "Sweeps
// ready and active features and approved non-terminal tasks." The filter
// lives here rather than inside mutate.BasisDrift/checkBasis, because
// accepted_against is never cleared at done or cancelled, and an unfiltered
// sweep would misfile stale_spec's business under this marker instead.
var activeBasisFeatureStatuses = map[string]bool{"ready": true, "active": true}
var activeBasisTaskStatuses = map[string]bool{"ready": true, "in_progress": true, "check": true, "in_review": true}

// sweepActiveBasis walks the ready/active features and the ready/
// in_progress/check/in_review tasks, calling mutate.BasisDrift per node (the
// same predicate start/check/submit refuse on) and folding each result into
// rep. Iteration is over sorted ids so the Report's slice order is
// deterministic across runs.
//
// A task gets a SECOND, separate check: reportTaskOwnBasis, after its
// parent-walk result (reportActiveBasis) is already recorded. BasisDrift's own
// task branch checks the parent feature first and stops there as soon as it
// drifts, which is right for a guard refusing one thing at a time but wrong for
// a sweep that wants to name every drifted node: without the second call, a
// task whose OWN spec drifted stays invisible whenever its parent feature ALSO
// drifted.
//
// Compared and Coverage are both decided here, ONCE PER NODE, from the
// COMBINED result of every half-check that node needed: a feature has one half,
// a task has two, and either way this loop is the only place that bumps them.
// Earlier this task bumped Coverage inside classifyBasisDrift itself, once per
// CALL rather than once per NODE, so a clean task (two calls, both succeeding)
// counted twice in "checked" while Compared counted it once, and a task under
// an unreadable parent could land in both "checked" (its own pin, read fine)
// and "unresolvable" (its parent's, not read at all) at the same time. Neither
// mistake is possible once the decision is made exactly once, here, from the
// AND of both halves.
func sweepActiveBasis(t *store.Tree, rev vcs.Revisions, rep *Report) {
	seen := map[string]bool{}

	for _, id := range sortedFeatureIDs(t) {
		f := t.Features[id]
		if !activeBasisFeatureStatuses[f.Status] {
			continue
		}
		ok, detail := reportActiveBasis(t, rev, rep, f, seen)
		finishActiveBasisNode(rep, f, ok, detail)
	}

	for _, id := range sortedTaskIDs(t) {
		tk := t.Tasks[id]
		if !activeBasisTaskStatuses[tk.Status] {
			continue
		}
		parentOK, parentDetail := reportActiveBasis(t, rev, rep, tk, seen)
		ownOK, ownDetail := reportTaskOwnBasis(t, rev, rep, tk, seen)
		finishActiveBasisNode(rep, tk, parentOK && ownOK, joinBasisDetails(parentDetail, ownDetail))
	}
}

// finishActiveBasisNode records Compared and Coverage for exactly one swept
// node, from the combined outcome of every half-check it needed (drift-
// review, Important 1). A false result also appends the node's ONE
// Uncovered row, built from whichever half(s) actually failed: a task whose
// parent-walk AND own check both came back unreadable gets ONE row naming
// both, not two identical-looking ones differing only in which half is
// named (resolved by merging rather than by
// adding a field: Coverage and Compared are already strictly per-node after
// this fix, and a per-node Uncovered list matches that same "one row per
// node" shape without a new field for later markers to have to carry too).
func finishActiveBasisNode(rep *Report, n store.Node, ok bool, detail string) {
	if ok {
		bumpCoverage(rep, MarkerActiveBasis, "checked")
		markCompared(rep, n.Noun(), n.NodeID())
		return
	}
	bumpCoverage(rep, MarkerActiveBasis, "unresolvable")
	rep.Uncovered = append(rep.Uncovered, Uncovered{
		Marker: MarkerActiveBasis,
		Noun:   n.Noun(),
		ID:     n.NodeID(),
		Key:    nodeKey(n),
		Reason: "basis_unreadable",
		Detail: detail,
	})
}

// joinBasisDetails merges the failing half-check details (empty strings from
// a half that succeeded are dropped) into the one sentence
// finishActiveBasisNode's single Uncovered row carries.
func joinBasisDetails(details ...string) string {
	var kept []string
	for _, d := range details {
		if d != "" {
			kept = append(kept, d)
		}
	}
	return strings.Join(kept, "; ")
}

// reportActiveBasis calls mutate.BasisDrift (the parent-walk comparison: a
// feature's own pair, or a task's parent-first-then-own pair) and folds the
// result through classifyBasisDrift.
func reportActiveBasis(t *store.Tree, rev vcs.Revisions, rep *Report, n store.Node, seen map[string]bool) (bool, string) {
	return classifyBasisDrift(t, rep, n, mutate.BasisDrift(t, n, rev), seen)
}

// reportTaskOwnBasis calls mutate.TaskOwnBasisDrift, the task-own-pin half
// BasisDrift's own task branch would also reach if its parent check had not
// already failed. Running it separately,
// AFTER the parent-walk result (reportActiveBasis) is already recorded, is
// what keeps a task's own drift from being masked whenever its parent
// feature has ALSO drifted. Deduping by the same key means the ordinary
// case, where the feature is clean and BasisDrift's own task branch already
// reached and reported the task's own drift, costs nothing beyond one
// redundant, deduped check.
func reportTaskOwnBasis(t *store.Tree, rev vcs.Revisions, rep *Report, tk *store.Task, seen map[string]bool) (bool, string) {
	return classifyBasisDrift(t, rep, tk, mutate.TaskOwnBasisDrift(t, tk, rev), seen)
}

// classifyBasisDrift folds one BasisDrift-shaped comparison's outcome into
// rep's Candidates and reports back whether THIS HALF completed, plus, if it
// did not, the human detail naming what failed. It no longer touches
// Coverage or Compared itself (those are
// decided once per NODE, by the caller, from the combined result of every
// half a node needs, never once per comparison CALL). A nil error or a
// (possibly deduped) *ActiveBasisError both count as this half completing:
// the comparison itself ran either way, and only the duplicate Candidate
// ROW is what dedupe exists to suppress. Anything else is a git or
// filesystem fault reading an existing, in-principle-hashable path (or,
// since a node with no key to build a
// path from at all), which did not complete. swept is the node the
// comparison was run FOR, used to build the detail sentence: there is no
// more specific "reported" node in that case, unlike a *ActiveBasisError,
// whose own Noun/NodeID can name a node other than swept (mutate.BasisDrift's
// own doc comment, for a task's parent-walk).
func classifyBasisDrift(t *store.Tree, rep *Report, swept store.Node, err error, seen map[string]bool) (bool, string) {
	if err == nil {
		return true, ""
	}

	var abErr *mutate.ActiveBasisError
	if !errors.As(err, &abErr) {
		return false, fmt.Sprintf("could not read %s's working-tree basis: %s", swept.NodeID(), err)
	}

	key := abErr.Noun + ":" + abErr.NodeID
	if seen[key] {
		return true, ""
	}
	seen[key] = true

	reported := basisReportedNode(t, abErr.Noun, abErr.NodeID)
	detail, context := activeBasisDetail(abErr)
	rep.Candidates = append(rep.Candidates, Candidate{
		Marker:  MarkerActiveBasis,
		Noun:    abErr.Noun,
		ID:      abErr.NodeID,
		Key:     nodeKey(reported),
		Detail:  detail,
		Files:   abErr.Drifted,
		Since:   basisSince(reported),
		Context: context,
	})
	return true, ""
}

// activeBasisDetail renders one human sentence for an active_basis
// candidate, plus any Context. A pinned file checkBasis recorded in Missing
// was deleted, not merely edited, and the remedy differs: --refresh-basis
// re-hashes the working tree, which it cannot do for a file that is not
// there. A file that drifted but is still
// present rides as "changed"; when both kinds appear together, the deleted
// ones lead the sentence and the merely-changed ones ride beside it as
// Context, so neither fact is lost.
func activeBasisDetail(abErr *mutate.ActiveBasisError) (string, []string) {
	if len(abErr.Missing) == 0 {
		return fmt.Sprintf("%s changed since the accepted basis", strings.Join(abErr.Drifted, ", ")), nil
	}

	missing := map[string]bool{}
	for _, m := range abErr.Missing {
		missing[m] = true
	}
	var alsoChanged []string
	for _, d := range abErr.Drifted {
		if !missing[d] {
			alsoChanged = append(alsoChanged, d)
		}
	}

	detail := fmt.Sprintf("%s deleted since the accepted basis", strings.Join(abErr.Missing, ", "))
	var context []string
	if len(alsoChanged) > 0 {
		context = []string{fmt.Sprintf("%s also changed since the accepted basis", strings.Join(alsoChanged, ", "))}
	}
	return detail, context
}

// basisReportedNode resolves the node an *ActiveBasisError names. That is
// not always the node the sweep was iterating (mutate.BasisDrift checks a
// task's parent feature first), so a Candidate's Key and Since read off the
// node the error actually names.
func basisReportedNode(t *store.Tree, noun, id string) store.Node {
	switch noun {
	case store.NounFeature:
		if f := t.Features[id]; f != nil {
			return f
		}
	case store.NounTask:
		if tk := t.Tasks[id]; tk != nil {
			return tk
		}
	}
	return nil
}

// basisSince reads the reported node's own AcceptedAgainst.Commit, the
// anchor a candidate's since-when reads from (artifact-schema.md, Accepted
// basis). A zero anchor (a pre-anchor record) is nil, not evidence the sweep
// failed to check the node: the comparison still ran.
func basisSince(n store.Node) *store.RevisionRef {
	switch v := n.(type) {
	case *store.Feature:
		if v.AcceptedAgainst.Commit.Value == "" {
			return nil
		}
		c := v.AcceptedAgainst.Commit
		return &c
	case *store.Task:
		if v.AcceptedAgainst == nil || v.AcceptedAgainst.Commit.Value == "" {
			return nil
		}
		c := v.AcceptedAgainst.Commit
		return &c
	default:
		return nil
	}
}

func sortedFeatureIDs(t *store.Tree) []string {
	ids := make([]string, 0, len(t.Features))
	for id := range t.Features {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedTaskIDs(t *store.Tree) []string {
	ids := make([]string, 0, len(t.Tasks))
	for id := range t.Tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// activeBasisForNode is ForNode's active_basis entry: the identical per-node
// body sweepActiveBasis's own two loops run above, lifted out so both
// callers share it rather than the sweep being the only place it exists. The
// status filters (activeBasisFeatureStatuses/activeBasisTaskStatuses) are
// reused verbatim, so a node the sweep would never have reached (a draft
// feature, a done task) yields nothing here either. seen is fresh per call: a
// single node's own dedupe never needs to survive past this one call, unlike
// the sweep's tree-wide seen map, which dedupes a task's parent-walk hit
// against its feature's own report across the WHOLE loop.
func activeBasisForNode(t *store.Tree, rev vcs.Revisions, rep *Report, n store.Node) {
	seen := map[string]bool{}
	switch v := n.(type) {
	case *store.Feature:
		if !activeBasisFeatureStatuses[v.Status] {
			return
		}
		ok, detail := reportActiveBasis(t, rev, rep, v, seen)
		finishActiveBasisNode(rep, v, ok, detail)
	case *store.Task:
		if !activeBasisTaskStatuses[v.Status] {
			return
		}
		parentOK, parentDetail := reportActiveBasis(t, rev, rep, v, seen)
		ownOK, ownDetail := reportTaskOwnBasis(t, rev, rep, v, seen)
		finishActiveBasisNode(rep, v, parentOK && ownOK, joinBasisDetails(parentDetail, ownDetail))
	}
	// An epic or a question carries no accepted_against at all; neither
	// matches a case above, so both fall through to nothing, the same
	// silence the sweep's own two loops (features, then tasks) already give
	// either noun.
}

// --- answered_question ---

// answeredQuestionTerminalStatuses are the feature statuses OUT of the
// sweep, written as the excluded set because "non-terminal" is everything
// but these two and naming the short list is what the schema asks for
// explicitly (artifact-schema.md, the answered_question bullet: "Sweeps
// non-terminal nodes; the done case is deliberately out, because its fix is an
// amendment, a human judgment").
var answeredQuestionTerminalStatuses = map[string]bool{"done": true, "cancelled": true}

// sweepAnsweredQuestion walks non-terminal features alone, never tasks.
// assumption_basis lives on Feature only (mutate.AssumptionBasis is
// feature-only), and
// mutate.UnreviewedAssumptions always walks a task up to its parent feature
// regardless of which node it is handed (assumptionFeatureFor), so a task
// hit would only duplicate its parent feature's own entry. Sweeping features
// alone therefore already covers every assumption_basis entry in the tree.
// Compared and Coverage are both incremented inside reportAnsweredQuestion
// itself, once per feature and only once every entry on it has been read,
// which is what lets that function decide whether the whole node counts as
// checked or unresolvable.
func sweepAnsweredQuestion(t *store.Tree, rev vcs.Revisions, rep *Report) {
	for _, id := range sortedFeatureIDs(t) {
		f := t.Features[id]
		if answeredQuestionTerminalStatuses[f.Status] {
			continue
		}
		reportAnsweredQuestion(t, f, rep)
	}
}

// reportAnsweredQuestion folds one feature's WHOLE assumption_basis into
// rep. It reads the feature's own AssumptionBasis keys directly
// rather than relying on
// mutate.UnreviewedAssumptions alone, because that predicate deliberately
// skips a dangling question reference in silence (Task 5's contract: "not
// this predicate's business... the drift engine's own sweep reports it as
// Uncovered"), so this sweep is the one place that dangling case is actually
// noticed.
//
// Coverage and Compared count this FEATURE once, not once per entry
// ("answered_question's Coverage buckets
// per FEATURE, not per entry, so units match Compared"): a feature carrying
// even one dangling reference is "unresolvable" as a whole and is not
// counted in Compared, even when one of its OTHER entries also produced a
// real Candidate, because a node with any unresolvable part was not fully
// checked. Only a feature with no dangling entries at all is "checked" and
// counted, whether or not it also produced Candidates.
func reportAnsweredQuestion(t *store.Tree, f *store.Feature, rep *Report) {
	unreviewed := map[string]mutate.UnreviewedAssumption{}
	for _, u := range mutate.UnreviewedAssumptions(t, f) {
		unreviewed[u.QuestionID] = u
	}

	unresolvable := false
	for _, qid := range sortedAssumptionIDs(f.AssumptionBasis) {
		q := t.Questions[qid]
		if q == nil {
			unresolvable = true
			rep.Uncovered = append(rep.Uncovered, Uncovered{
				Marker: MarkerAnsweredQuestion,
				Noun:   store.NounFeature,
				ID:     f.ID,
				Key:    nodeKey(f),
				Reason: "unknown_question",
				Detail: fmt.Sprintf("assumption_basis names %s, which the question ledger no longer carries", qid),
			})
			continue
		}

		u, isUnreviewed := unreviewed[qid]
		if !isUnreviewed {
			continue
		}
		rep.Candidates = append(rep.Candidates, Candidate{
			Marker:   MarkerAnsweredQuestion,
			Noun:     store.NounFeature,
			ID:       f.ID,
			Key:      nodeKey(f),
			Question: u.QuestionID,
			Detail:   fmt.Sprintf("%s %s (%s) was answered and has not been reviewed in", u.QuestionID, u.Title, u.Statement),
			// Since dates when the ANSWER landed, not when the plan was
			// approved: the drift
			// began the moment the question was answered, and the question's
			// own AnswerRevision is already a recorded commit ref for that
			// moment. The feature's own approval anchor dates the plan
			// instead, the wrong clock for this marker.
			Since: questionSince(q),
		})
	}

	if unresolvable {
		bumpCoverage(rep, MarkerAnsweredQuestion, "unresolvable")
		return
	}
	bumpCoverage(rep, MarkerAnsweredQuestion, "checked")
	markCompared(rep, store.NounFeature, f.ID)
}

// questionSince reads a question's own AnswerRevision, nil unless it is
// git_commit-kind. AnswerRevision is written by mutate.AnswerQuestion through
// gateAgainst (check.go), which returns CurrentCommit() on a clean tree but a
// WorktreeFingerprint() sha256 otherwise, and answering mid-flight, tree
// dirty, is the normal case, not the exception. A sha256 fingerprint is not a
// commit a human or an agent can diff from the way every other marker's short
// git sha is, so it is withheld here rather than rendered as if it were one
// (Since can be a fingerprint). Checking Kind rather than Value
// also folds in the pre-anchor case for free: a hand-authored ledger entry
// carrying status: answered with no answer_revision at all
// (mutate.ReviewAssumption's own doc comment) has Kind == "" too, the same
// "nothing to date it by, not a failed check" reading basisSince applies to a
// zero AcceptedAgainst.Commit one grain up.
func questionSince(q *store.Question) *store.RevisionRef {
	if q == nil || q.AnswerRevision.Kind != store.KindGitCommit {
		return nil
	}
	rev := q.AnswerRevision
	return &rev
}

func sortedAssumptionIDs(basis store.AssumptionBasis) []string {
	ids := make([]string, 0, len(basis))
	for qid := range basis {
		ids = append(ids, qid)
	}
	sort.Strings(ids)
	return ids
}

// answeredQuestionForNode is ForNode's answered_question entry. A feature is
// checked directly, reusing reportAnsweredQuestion verbatim (the sweep's own
// per-feature body above). sweepAnsweredQuestion never sweeps a task
// directly (assumption_basis lives on Feature alone), but a task's OWN badge
// still has to answer "would submitting THIS task refuse", which reads its
// PARENT feature's review state, the identical walk-up
// mutate.UnreviewedAssumptions and guardSubmitAssumptions both make for a
// submitting task (mutate/transition.go's own assumptionFeatureFor,
// unexported there and mirrored here in miniature rather than exported,
// since this switch is the only line in this package that needs it). The
// feature-status filter still applies to that resolved parent: a task under
// a done or cancelled feature answers nothing here either, the identical
// "the done case is deliberately out" rule the sweep applies to the feature
// directly one section up.
//
// A task also gates on its OWN terminal status (done or cancelled),
// symmetric with activeBasisForNode's own task branch, which checks the
// swept node's own status (activeBasisTaskStatuses) beside the parent walk
// it makes: a task that has already finished or been
// cancelled has no submit left for an unreviewed assumption to ever refuse,
// so a badge on its own page would name a fact its own page can do nothing
// about, however live the parent feature's assumption still is for every
// OTHER, non-terminal task under it.
func answeredQuestionForNode(t *store.Tree, rep *Report, n store.Node) {
	var feat *store.Feature
	switch v := n.(type) {
	case *store.Feature:
		feat = v
	case *store.Task:
		if answeredQuestionTerminalStatuses[v.Status] {
			return
		}
		feat = t.Features[v.FeatureID]
	}
	if feat == nil || answeredQuestionTerminalStatuses[feat.Status] {
		return
	}
	reportAnsweredQuestion(t, feat, rep)
}

func sortedEpicIDs(t *store.Tree) []string {
	ids := make([]string, 0, len(t.Epics))
	for id := range t.Epics {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// statPath reports whether root+rel exists, distinguishing a clean "not
// there" (present=false, err=nil) from an unexpected stat failure
// (permission, a symlink loop, ...): the first is the vanish both
// stale_spec's surface leg and missing_surface turn a candidate on, the
// second is a node neither marker can finish checking (artifact-schema.md,
// Drift control hooks: "a node the sweep could not check is reported
// uncovered... never folded into silence").
func statPath(root, rel string) (present bool, err error) {
	return store.PathExistsNoSymlinks(root, filepath.Join(root, rel))
}

// --- stale_spec ---

// staleSpecFeatureExempt reports whether f is exempt from the stale_spec
// sweep: a feature superseded by another is drifted by construction, a human
// already recorded the replacement, so flagging its now-stale completion
// record would only repeat a decision that has already been made
// (artifact-schema.md, Drift control hooks: stale_spec, "the one drifted by
// construction").
//
// Exempt on SupersededBy's presence alone, including a dangling id that no
// longer resolves to a live feature. The principled line: Uncovered means the
// MARKER could not do its own job, the way a dangling assumption_basis qid
// breaks answered_question's own comparison (reportAnsweredQuestion's own
// unknown_question arm, one marker up); exemption means a human recorded a
// decision, and the decision here is the supersession record's PRESENCE, not
// whether its target still resolves. Noticing a dangling superseded_by id is
// a validation surface's job, not this sweep's.
func staleSpecFeatureExempt(f *store.Feature) bool {
	return f.SupersededBy != nil
}

// sweepStaleSpec walks done features, then done epics, folding each
// comparison into rep. A superseded feature is exempted before
// VerifiedAgainst is ever opened, unconditionally once a human's decision is
// recorded (the exemption principle, artifact-schema.md's closing paragraph
// on Drift control hooks): the done-status filter above it already guarantees
// a superseded feature counted here was otherwise in scope, since supersession
// coexists with status "done" rather than replacing it.
//
// An archived epic is different in exactly this respect: archived REPLACES
// done (the lifecycle table's one edge into it is {From: "done", Verb:
// "archive"}, internal/core/lifecycle/table.go), so by the time this loop
// sees status "archived" there is no surviving "done" to filter on the way
// the feature loop above does. The exemption is gated on VerifiedAgainst
// instead: a real done crossing populates it (crossEpicDone's own
// recordEpicVerified) and archive never touches it, so an epic hand-edited
// straight from draft or ready to archived, skipping done entirely, never
// reachable through the archive verb itself, carries no completion record
// and is never otherwise in this sweep's done set. Counting it anyway would
// be the identical mistake sweepMissingSurface's and sweepOrphanEval's own
// cancelled arms already avoid one section over: count the exemption only
// when the node would otherwise have qualified.
//
// Coverage counts NODES here, the same unit Compared counts in (generalized to
// every marker in this package): a done feature or epic is one unit whether its
// comparison spans one leg or three.
func sweepStaleSpec(t *store.Tree, rev vcs.Revisions, rep *Report) {
	for _, id := range sortedFeatureIDs(t) {
		f := t.Features[id]
		if f.Status != "done" {
			continue
		}
		if staleSpecFeatureExempt(f) {
			rep.Exempted[MarkerStaleSpec]++
			continue
		}
		reportStaleSpecFeature(t, rev, rep, f)
	}

	for _, id := range sortedEpicIDs(t) {
		e := t.Epics[id]
		if e.Status == "archived" {
			if !verifiedAgainstEmpty(e.VerifiedAgainst) {
				rep.Exempted[MarkerStaleSpec]++
			}
			continue
		}
		if e.Status != "done" {
			continue
		}
		reportStaleSpecEpic(t, rev, rep, e)
	}
}

// verifiedAgainstEmpty reports whether v was never populated at all: every
// leg's Value is empty and Surfaces was never even initialized to the empty
// list recordVerified/recordEpicVerified always write at a real crossing.
// Checking every field, not Surfaces alone, matters: a record could in
// principle carry real spec/commit values yet still read Surfaces == nil (a
// hand-authored fixture missing just that key, or a future writer that drops
// it), and keying only on Surfaces would misdescribe that node as carrying no
// completion record at all, when most of one is actually there. Eval is checked
// too for symmetry even though an epic's own Eval is always zero regardless:
// that fact alone can never flip this answer, since Spec, Commit, and Surfaces
// still all have to agree for the whole expression to read empty.
func verifiedAgainstEmpty(v store.VerifiedAgainst) bool {
	return v.Spec.Value == "" && v.Eval.Value == "" && v.Commit.Value == "" && v.Surfaces == nil
}

// staleSpecItem is one file staleSpecFinding names twice over: path is
// always the full repo-relative path (what Files carries), and label is what
// the Detail sentence names it by, which for the spec/eval leg is a shorter
// basename (the two are allowed to
// differ, and here they do, on purpose).
type staleSpecItem struct {
	path  string
	label string
}

func itemPaths(items []staleSpecItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.path
	}
	return out
}

func itemLabels(items []staleSpecItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.label
	}
	return out
}

// staleSpecFinding accumulates one done node's whole stale_spec comparison
// before a single Candidate (or none) is built from it, the same one-
// candidate-per-node shape checkBasis already gives active_basis: a done
// feature's spec, eval, and surface legs are three separate comparisons that
// still describe one drifted node, not three.
//
// vanished and changed each carry one staleSpecItem per file. Every entry's
// path is the full repo-relative path regardless of which leg produced it
// (Task 10 groups and resolves Files
// entries uniformly, so a spec/eval leg basename mixed among surface leg full
// paths would be a namespace Task 10 could not resolve without first knowing
// which leg produced which entry). Only the label, used for the Detail
// sentence alone, differs by leg: the spec/eval leg's own basename, since
// those files always live in the node's own folder and the longer path is
// only noise there; the surface leg's own full path unchanged, since a
// surface can point anywhere and a basename alone could name any number of
// files across the repo. context carries the surface leg's tree-hash-moved
// note, which rides only when vanished or changed is also non-empty: the
// tree-hash difference has nothing to ride beside when nothing vanished or
// changed, so on its own it is not reported at all (the vanish-only rule).
type staleSpecFinding struct {
	vanished []staleSpecItem
	changed  []staleSpecItem
	context  []string
}

func (f staleSpecFinding) empty() bool {
	return len(f.vanished) == 0 && len(f.changed) == 0
}

// reportStaleSpecFeature compares one done feature's verified_against against
// the current working tree and folds the result into rep. A feature with no
// completion record at all is Uncovered rather than clean (verifiedAgainstEmpty):
// recordVerified always sets Surfaces to a non-nil (possibly empty) list at
// the done crossing, so a genuinely empty record means this feature reached
// done some other way (a hand-authored file, a crossing predating this
// record), and a clean report would assert a comparison that never actually
// ran.
func reportStaleSpecFeature(t *store.Tree, rev vcs.Revisions, rep *Report, f *store.Feature) {
	if verifiedAgainstEmpty(f.VerifiedAgainst) {
		staleSpecUncovered(rep, f, "no_completion_record",
			"done but carries no verified_against completion record")
		return
	}

	var find staleSpecFinding

	if ok, detail := staleSpecFileLeg(rev, t.Root, f.VerifiedAgainst.Spec, &find); !ok {
		staleSpecUncovered(rep, f, "spec_unreadable", detail)
		return
	}
	if ok, detail := staleSpecFileLeg(rev, t.Root, f.VerifiedAgainst.Eval, &find); !ok {
		staleSpecUncovered(rep, f, "eval_unreadable", detail)
		return
	}

	failures := staleSpecSurfacesLeg(rev, t.Root, f.VerifiedAgainst.Surfaces, &find)
	finishStaleSpec(rep, f, find, failures)
}

// reportStaleSpecEpic is reportStaleSpecFeature's epic-grain twin. An epic
// carries no eval file of its own (artifact-schema.md, Epics: "its acceptance
// bar is the sum of its features' contracts"), so there is no eval leg to
// compare here at all, structurally, rather than by a zero-value skip.
func reportStaleSpecEpic(t *store.Tree, rev vcs.Revisions, rep *Report, e *store.Epic) {
	if verifiedAgainstEmpty(e.VerifiedAgainst) {
		staleSpecUncovered(rep, e, "no_completion_record",
			"done but carries no verified_against completion record")
		return
	}

	var find staleSpecFinding

	if ok, detail := staleSpecFileLeg(rev, t.Root, e.VerifiedAgainst.Spec, &find); !ok {
		staleSpecUncovered(rep, e, "spec_unreadable", detail)
		return
	}

	failures := staleSpecSurfacesLeg(rev, t.Root, e.VerifiedAgainst.Surfaces, &find)
	finishStaleSpec(rep, e, find, failures)
}

// staleSpecFileLeg compares one verified spec or eval file against the
// current working tree through mutate.CompareWorktreeBlob. recorded's value is
// the integration commit's committed blob (recordVerified reads it through
// committedBlobRef), and the working tree is read deliberately: stale_spec
// asks whether the checkout has moved off what was verified. The working-tree
// side is hashed the way `git add` would store it, checkin conversion and clean
// filters applied, so a CRLF checkout under text=auto or an LFS file that was
// not edited compares equal to its committed blob. It reads the path to compare
// straight off recorded.Path, the exact relPath recordVerified hashed.
// A recorded ref with no Value is skipped
// (an epic's Eval leg is always zero; a pre-Task-2 record may carry no spec pin
// at all), since there is nothing to compare it against. ok is false only when
// the file exists but could not be hashed for some reason other than absence;
// the caller reports that as Uncovered, with its own reason,
// rather than guessing which way the
// comparison would have gone.
func staleSpecFileLeg(rev vcs.Revisions, root string, recorded store.RevisionRef, find *staleSpecFinding) (ok bool, unreadableDetail string) {
	if recorded.Value == "" {
		return true, ""
	}
	vanished, changed, err := mutate.CompareWorktreeBlob(rev, root, recorded.Path, recorded.Value)
	if err != nil {
		return false, fmt.Sprintf("could not read %s: %s", recorded.Path, err)
	}
	item := staleSpecItem{path: recorded.Path, label: filepath.Base(recorded.Path)}
	if vanished {
		find.vanished = append(find.vanished, item)
	}
	if changed {
		find.changed = append(find.changed, item)
	}
	return true, ""
}

// staleSpecSurfacesLeg walks every recorded surface fingerprint, accumulating
// into find whatever it can determine and returning every hard-failure
// detail it hits along the way rather than stopping at the first one:
// a two-surface feature where
// fingerprint A is fully vanished and fingerprint B is unreadable must still
// report A as an actionable candidate, not lose it behind B's failure. The
// caller lands the node in Uncovered too when the returned failures is
// non-empty (never Compared, matching every other uncovered node in this
// package), so the two lists are not mutually exclusive in this one case, on
// purpose: an agent fixing what is already known should not have to wait on
// a human clearing an unrelated permission fault first.
//
// Missing inventory entries are checked BEFORE the hash read (TreeHash for a
// git_tree fingerprint, BlobHash for a git_blob) is ever called for a given
// fingerprint, and a fingerprint with any missing entries never reaches that
// hash read at all. This ordering is the vanish rule's whole defining boundary:
// a directory whose every recorded
// file is gone is a plain candidate naming every one of them, nothing
// uncovered, because a hash read on a path that no longer resolves at HEAD
// would otherwise read as a hard failure rather than the total vanish it
// actually is. TestStaleSpecWholeSurfaceDirectoryVanishIsOneCandidateNotUncovered
// is what defends this ordering; it fails the moment the two checks swap.
func staleSpecSurfacesLeg(rev vcs.Revisions, root string, surfaces []store.SurfaceFingerprint, find *staleSpecFinding) []string {
	var failures []string
	for _, sf := range surfaces {
		var missing []string
		hardFailed := false
		for _, inv := range sf.Inventory {
			present, err := statPath(root, inv)
			if err != nil {
				failures = append(failures, fmt.Sprintf("could not stat %s: %s", inv, err))
				hardFailed = true
				continue
			}
			if !present {
				missing = append(missing, inv)
			}
		}
		for _, m := range missing {
			find.vanished = append(find.vanished, staleSpecItem{path: m, label: m})
		}
		if hardFailed || len(missing) > 0 || sf.Value == "" {
			continue
		}
		read, target := rev.TreeHash, fmt.Sprintf("tree hash under %s", sf.Path)
		if sf.Kind == store.KindGitBlob {
			read, target = rev.BlobHash, fmt.Sprintf("blob hash of %s", sf.Path)
		}
		hash, err := read(sf.Path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("could not read the current %s: %s", target, err))
			continue
		}
		if "git:"+hash != sf.Value {
			// For a tree, per-file hashes are not recorded, so this cannot
			// name which file under the path moved: "N recorded files
			// changed" would be unearned precision. A blob fingerprint IS one
			// file, so it names it.
			changed := fmt.Sprintf("content changed under %s", sf.Path)
			if sf.Kind == store.KindGitBlob {
				changed = fmt.Sprintf("content changed at %s", sf.Path)
			}
			find.context = append(find.context, changed)
		}
	}
	return failures
}

// staleSpecUncovered appends one Uncovered row for n and bumps Coverage's
// unresolvable bucket. n is always a *store.Feature or *store.Epic, the two
// nouns stale_spec sweeps.
func staleSpecUncovered(rep *Report, n store.Node, reason, detail string) {
	bumpCoverage(rep, MarkerStaleSpec, "unresolvable")
	rep.Uncovered = append(rep.Uncovered, Uncovered{
		Marker: MarkerStaleSpec, Noun: n.Noun(), ID: n.NodeID(), Key: nodeKey(n),
		Reason: reason, Detail: detail,
	})
}

// finishStaleSpec records Compared/Coverage for one node whose spec and eval
// legs both completed, and, when find carries anything, appends one
// Candidate. failures is the surfaces leg's own accumulated hard-failure
// detail: non-empty lands the node in
// "unresolvable" rather than "checked" and out of Compared, but a Candidate
// still gets appended alongside it whenever find is non-empty, since a
// failure on one fingerprint must never discard a real finding another
// fingerprint (or the spec/eval legs) already produced. A node can therefore
// appear in both Candidates and Uncovered in exactly this one case; every
// other Uncovered arm in this file still means the node produced no
// Candidate at all.
func finishStaleSpec(rep *Report, n store.Node, find staleSpecFinding, failures []string) {
	if len(failures) > 0 {
		bumpCoverage(rep, MarkerStaleSpec, "unresolvable")
		rep.Uncovered = append(rep.Uncovered, Uncovered{
			Marker: MarkerStaleSpec, Noun: n.Noun(), ID: n.NodeID(), Key: nodeKey(n),
			Reason: "surface_unreadable", Detail: strings.Join(failures, "; "),
		})
	} else {
		bumpCoverage(rep, MarkerStaleSpec, "checked")
		markCompared(rep, n.Noun(), n.NodeID())
	}
	if find.empty() {
		return
	}
	detail, context := staleSpecDetail(find)
	rep.Candidates = append(rep.Candidates, Candidate{
		Marker:  MarkerStaleSpec,
		Noun:    n.Noun(),
		ID:      n.NodeID(),
		Key:     nodeKey(n),
		Detail:  detail,
		Files:   append(itemPaths(find.vanished), itemPaths(find.changed)...),
		Since:   staleSpecSince(n),
		Context: context,
	})
}

// staleSpecDetail renders one human sentence from a non-empty finding, plus
// any Context, naming files by their Detail label (staleSpecItem.label), not
// their Files path: the two are allowed to differ, and here they do for the
// spec/eval leg. A vanished file leads
// the sentence, since it is the reason a candidate exists at all; a merely
// changed spec/eval file rides beside it as Context once something has also
// vanished (activeBasisDetail's own pattern, one marker over). When nothing
// vanished, the changed files stand as the whole sentence on their own,
// because unlike the surface leg, content drift on a done node's own spec or
// eval file IS itself a real stale_spec finding, not merely context.
func staleSpecDetail(find staleSpecFinding) (string, []string) {
	var detail string
	var context []string
	if len(find.vanished) > 0 {
		detail = fmt.Sprintf("%s deleted since it was verified at done", strings.Join(itemLabels(find.vanished), ", "))
		if len(find.changed) > 0 {
			context = append(context, fmt.Sprintf("%s also changed since it was verified at done", strings.Join(itemLabels(find.changed), ", ")))
		}
	} else {
		detail = fmt.Sprintf("%s changed since it was verified at done", strings.Join(itemLabels(find.changed), ", "))
	}
	context = append(context, find.context...)
	return detail, context
}

// staleSpecSince reads the completion record's own commit anchor, nil for a
// pre-anchor record (a done crossing recorded before Task 2 added Commit to
// VerifiedAgainst): the comparison still ran and still found what it found,
// there is simply nothing to date it by, the same reading basisSince gives
// AcceptedAgainst one marker up.
func staleSpecSince(n store.Node) *store.RevisionRef {
	switch v := n.(type) {
	case *store.Feature:
		if v.VerifiedAgainst.Commit.Value == "" {
			return nil
		}
		c := v.VerifiedAgainst.Commit
		return &c
	case *store.Epic:
		if v.VerifiedAgainst.Commit.Value == "" {
			return nil
		}
		c := v.VerifiedAgainst.Commit
		return &c
	default:
		return nil
	}
}

// staleSpecForNode is ForNode's stale_spec entry: a done feature not exempt
// by supersession, or a done epic, reusing reportStaleSpecFeature/
// reportStaleSpecEpic verbatim, the sweep's own per-node bodies above. An
// archived epic never reaches reportStaleSpecEpic here either, though for a
// simpler reason than the sweep's own two-step exempt-then-status-filter:
// archived is never "done", so the single status check below already
// excludes it. The sweep's separate archived arm exists only to COUNT the
// exemption in Exempted, a whole-sweep bookkeeping concept ForNode has no
// per-node analog for and does not return.
func staleSpecForNode(t *store.Tree, rev vcs.Revisions, rep *Report, n store.Node) {
	switch v := n.(type) {
	case *store.Feature:
		if v.Status != "done" || staleSpecFeatureExempt(v) {
			return
		}
		reportStaleSpecFeature(t, rev, rep, v)
	case *store.Epic:
		if v.Status != "done" {
			return
		}
		reportStaleSpecEpic(t, rev, rep, v)
	}
}

// --- missing_surface ---

// missingSurfaceFeatureOutOfScope and missingSurfaceEpicOutOfScope are the
// statuses this sweep never visits and never counts, in either list: each
// status belongs to a different concern (done is stale_spec's business; an
// archived epic is a decision already closed upstream of drift), so folding
// either in here would misfile that business under the wrong marker, echoing
// active_basis's own status-filter rationale one section up.
var missingSurfaceFeatureOutOfScope = map[string]bool{"done": true}
var missingSurfaceEpicOutOfScope = map[string]bool{"done": true, "archived": true}

// sweepMissingSurface walks every feature, then every epic, in id order. A
// cancelled node is exempt and counted only when it carries a surface:
// the marker's whole scope is nodes
// CARRYING a surface, so a cancelled node that never recorded one was never
// in that scope to begin with, and counting it anyway would inflate the
// footer for a set it never joined, asymmetrically with the silent
// out-of-scope every non-cancelled surfaceless node already gets below.
//
// Coverage counts NODES here too, matching Compared and stale_spec's own unit
// one section up: a feature or epic with several surface paths is still one
// unit, whether one path vanished or three did.
func sweepMissingSurface(t *store.Tree, rep *Report) {
	for _, id := range sortedFeatureIDs(t) {
		f := t.Features[id]
		if f.Status == "cancelled" {
			if len(f.Surface.Paths) > 0 {
				rep.Exempted[MarkerMissingSurface]++
			}
			continue
		}
		if missingSurfaceFeatureOutOfScope[f.Status] || len(f.Surface.Paths) == 0 {
			continue
		}
		reportMissingSurface(t, rep, f, f.Surface.Paths)
	}

	for _, id := range sortedEpicIDs(t) {
		e := t.Epics[id]
		if e.Status == "cancelled" {
			if len(e.Surface.Paths) > 0 {
				rep.Exempted[MarkerMissingSurface]++
			}
			continue
		}
		if missingSurfaceEpicOutOfScope[e.Status] || len(e.Surface.Paths) == 0 {
			continue
		}
		reportMissingSurface(t, rep, e, e.Surface.Paths)
	}
}

// reportMissingSurface stats every path n recorded and folds the result into
// rep: a path that is gone is the candidate the vanish rule exists for
// (artifact-schema.md, missing_surface). Since is never set at all, unlike
// stale_spec's completion-record anchor: Surface carries no revision of its
// own for `tiller link --surface` to have stamped (SurfaceResult, surface.go,
// has no commit or timestamp field to read one from), so there is nothing
// honest to date a candidate by here.
func reportMissingSurface(t *store.Tree, rep *Report, n store.Node, paths []string) {
	var missing []string
	for _, p := range paths {
		present, err := statPath(t.Root, p)
		if err != nil {
			bumpCoverage(rep, MarkerMissingSurface, "unresolvable")
			rep.Uncovered = append(rep.Uncovered, Uncovered{
				Marker: MarkerMissingSurface, Noun: n.Noun(), ID: n.NodeID(), Key: nodeKey(n),
				Reason: "surface_unreadable",
				Detail: fmt.Sprintf("could not stat %s: %s", p, err),
			})
			return
		}
		if !present {
			missing = append(missing, p)
		}
	}

	bumpCoverage(rep, MarkerMissingSurface, "checked")
	markCompared(rep, n.Noun(), n.NodeID())
	if len(missing) == 0 {
		return
	}
	rep.Candidates = append(rep.Candidates, Candidate{
		Marker: MarkerMissingSurface,
		Noun:   n.Noun(),
		ID:     n.NodeID(),
		Key:    nodeKey(n),
		Detail: fmt.Sprintf("%s no longer exists in the working tree", strings.Join(missing, ", ")),
		Files:  missing,
	})
}

// missingSurfaceForNode is ForNode's missing_surface entry: a feature or epic
// in scope (not cancelled, not done/archived, carrying at least one recorded
// surface path), reusing reportMissingSurface verbatim, the sweep's own
// per-node body above. Both branches fold "cancelled" straight into the
// return rather than the sweep's own separate exempt-and-count arm ahead of
// it: the candidate-production answer for a cancelled node is identical
// either way (nothing), and ForNode has no per-node Exempted count for the
// sweep's distinction to attach to.
func missingSurfaceForNode(t *store.Tree, rep *Report, n store.Node) {
	switch v := n.(type) {
	case *store.Feature:
		if v.Status == "cancelled" || missingSurfaceFeatureOutOfScope[v.Status] || len(v.Surface.Paths) == 0 {
			return
		}
		reportMissingSurface(t, rep, v, v.Surface.Paths)
	case *store.Epic:
		if v.Status == "cancelled" || missingSurfaceEpicOutOfScope[v.Status] || len(v.Surface.Paths) == 0 {
			return
		}
		reportMissingSurface(t, rep, v, v.Surface.Paths)
	}
}

// --- orphan_eval ---

// acceptedAgainstEmpty reports whether a is the zero value: no approval has
// ever recorded a basis. It is BasisRefs's own version of
// verifiedAgainstEmpty, the zero-struct test orphan_eval's preserve clock
// needs and nothing before it did. BasisRefs
// carries no Surfaces field to check (accepted_against pins a spec and an
// eval blob only; surface fingerprints are a done-time concept), so every leg
// here is a RevisionRef and the zero check is Value alone on each.
func acceptedAgainstEmpty(a store.BasisRefs) bool {
	return a.Spec.Value == "" && a.Eval.Value == "" && a.Commit.Value == ""
}

// orphanEvalItem is one criterion's missing ref: the criterion id (so
// Detail, and Task 10's later grouping, can name which criterion it belongs
// to without re-opening the eval file) and
// which polarity clock it was checked under, since a candidate that mixes
// both clocks anchors Since to whichever one is earlier (orphanEvalSince).
type orphanEvalItem struct {
	criterionID string
	ref         string
	clock       string // store.PolarityPreserve or store.PolarityIntroduce
}

// orphanEvalFinding accumulates one feature's whole orphan_eval comparison
// before a single Candidate (or none) is built from it: several criteria,
// and even two independent checks on one preserve criterion (its binding and
// its baseline), still describe one drifted node, the same one-candidate-
// per-node shape staleSpecFinding gives a multi-leg comparison one marker
// over.
type orphanEvalFinding struct {
	missing []orphanEvalItem
}

func (f orphanEvalFinding) empty() bool { return len(f.missing) == 0 }

// sweepOrphanEval walks every feature carrying an eval file whose polarity
// clock has started for at least one criterion's own polarity: a non-zero
// AcceptedAgainst (the preserve clock) or a non-zero VerifiedAgainst (the
// introduce clock). A feature with no eval file, or with BOTH clocks still
// zero (never approved), sits outside this marker's sweep set entirely, the
// same way a draft feature sits outside active_basis's own status filter:
// there is nothing yet a human has agreed the contract should be checked
// against (artifact-schema.md, the orphan_eval bullet).
//
// A cancelled feature is exempt and counted in Exempted, but ONLY when it is
// otherwise in this marker's scope (an eval file plus a started clock), the
// identical gating sweepMissingSurface already applies to its own cancelled
// check (drift.go's own
// sweepMissingSurface): the marker's whole scope is nodes it would otherwise
// have compared, so a cancelled feature that never carried an eval file, or
// carried one but never got approved, was never in that scope to begin with,
// and counting it anyway would inflate the footer for a set it never joined.
// Without this, a human's cancel decision left a node the sweep still walked
// like a live one, producing an actionable-looking candidate for work
// already taken off the table, one that could never self-clear (the
// schema's exemption paragraph, artifact-schema.md: "a node is exempt from a
// sweep only when a human recorded a decision that makes the drift
// expected... every exemption is counted in the report's footer"; this
// marker's own brief never named cancelled explicitly, which is the gap this
// fixes).
//
// Coverage counts CRITERIA here, not nodes, the one marker in this file that
// does not (documented at Report.Coverage): one feature's contract can hold
// several independently-checkable refs, and a preserve criterion can
// independently contribute a second check for its baseline artifact, so the
// bucket that keeps the path-shaped rule's conservatism honest (task-9-
// brief.md) has to count at that finer grain. Compared still
// counts NODES, matching every other marker: one bump per feature, in
// finishOrphanEval, after every criterion on it has been read.
func sweepOrphanEval(t *store.Tree, rep *Report) {
	for _, id := range sortedFeatureIDs(t) {
		f := t.Features[id]
		if !f.HasEval {
			continue
		}
		inScope := !acceptedAgainstEmpty(f.AcceptedAgainst) || !verifiedAgainstEmpty(f.VerifiedAgainst)
		if f.Status == "cancelled" {
			if inScope {
				rep.Exempted[MarkerOrphanEval]++
			}
			continue
		}
		if !inScope {
			continue
		}
		reportOrphanEval(t, rep, f)
	}
}

// reportOrphanEval loads and validates one feature's eval file exactly the
// way serve's evalPayload does (internal/serve/node.go: store.LoadEvalFile
// then ValidateContract), then walks its criteria. A feature with no key
// cannot have its eval path resolved at all (gen.EvalPath derives it from
// the feature's own folder, which the key names), and a load or validate
// failure means there is no well-formed contract to check criteria against;
// either way the whole node is Uncovered with reason eval_unreadable and
// none of its criteria are visited, matching serve's own choice to fold a
// load failure and a validation failure into the identical error view for
// the identical reason: neither one is a checkable contract.
func reportOrphanEval(t *store.Tree, rep *Report, f *store.Feature) {
	if f.Key == nil {
		orphanEvalUncovered(rep, f, "feature carries no key, so its eval file path cannot be resolved")
		return
	}

	// EvalPath answers ok=false only for a noun that carries no eval concept
	// at all (a task, an epic, a question); called with a *store.Feature, as
	// every caller in this sweep does, it always answers true, so the bool is
	// not re-checked here.
	rel, _ := gen.EvalPath(t, f)
	ef, err := store.LoadEvalFile(t.Root, filepath.FromSlash(rel))
	if err != nil {
		orphanEvalUncovered(rep, f, fmt.Sprintf("could not read %s: %s", rel, err))
		return
	}
	if err := ef.ValidateContract(); err != nil {
		orphanEvalUncovered(rep, f, fmt.Sprintf("%s does not carry a well-formed contract: %s", rel, err))
		return
	}

	preserveStarted := !acceptedAgainstEmpty(f.AcceptedAgainst)
	introduceStarted := !verifiedAgainstEmpty(f.VerifiedAgainst)

	var find orphanEvalFinding
	var failures []string

	for _, c := range ef.Criteria {
		switch c.Polarity {
		case store.PolarityPreserve:
			if !preserveStarted {
				bumpCoverage(rep, MarkerOrphanEval, "not_yet_in_scope")
				continue
			}
		case store.PolarityIntroduce:
			if !introduceStarted {
				bumpCoverage(rep, MarkerOrphanEval, "not_yet_in_scope")
				continue
			}
		default:
			// ValidateContract does not constrain polarity to these two
			// values, so a hand-edited file could carry a typo or an empty
			// string. Treating that as never-started is the same
			// conservative, never-guess line the path-shaped rule draws one
			// step down: there is no clock to read, so nothing is checked.
			bumpCoverage(rep, MarkerOrphanEval, "not_yet_in_scope")
			continue
		}

		failures = append(failures, orphanEvalCheckBinding(rep, t.Root, c, &find)...)
		if c.Polarity == store.PolarityPreserve && c.Baseline != nil {
			if artifact := strings.TrimSpace(c.Baseline.Artifact); artifact != "" {
				if failed, detail := orphanEvalCheckPath(rep, t.Root, c.ID, artifact, c.Polarity, baselineArtifactAdvice, &find); failed {
					failures = append(failures, detail)
				}
			}
		}
	}

	finishOrphanEval(rep, f, find, failures)
}

// orphanEvalCheckBinding classifies one already-in-scope criterion's OWN
// binding (never its baseline; the caller checks that separately, since a
// preserve criterion can contribute both). command, manual, and eval
// bindings carry nothing this marker can stat: a shell string offers
// nothing to stat and running it is check's job, and a human verification
// or a rubric packet are not files either, so all three count as
// out_of_scope (artifact-schema.md, orphan_eval). Each item of a test
// binding's ref list is path-shaped when it contains a path separator, checked
// one at a time via orphanEvalCheckPath so every missing file is its own
// item; otherwise it counts unresolvable, whether the item is a bare test
// name or id (an argument to the tests runner, not a path), and a binding
// with no ref and only run set counts unresolvable once (a run string is no
// more a path than a command's is). Grepping for either would be guessing.
// It returns one detail per ref that
// could not be read.
func orphanEvalCheckBinding(rep *Report, root string, c store.EvalCriterion, find *orphanEvalFinding) (failures []string) {
	switch c.Binding.Type {
	case store.BindingCommand, store.BindingManual, store.BindingEval:
		bumpCoverage(rep, MarkerOrphanEval, "out_of_scope")
	case store.BindingTest:
		if c.Binding.Ref.Empty() {
			bumpCoverage(rep, MarkerOrphanEval, "unresolvable")
			return nil
		}
		for _, item := range c.Binding.Ref {
			ref := strings.TrimSpace(item)
			if !evalref.IsPathShaped(ref) {
				bumpCoverage(rep, MarkerOrphanEval, "unresolvable")
				continue
			}
			if failed, detail := orphanEvalCheckPath(rep, root, c.ID, ref, c.Polarity, testRefAdvice, find); failed {
				failures = append(failures, detail)
			}
		}
	default:
		// ValidateContract already refused any other binding type before this
		// sweep ever opened the file; kept only so this switch never
		// silently falls through a value it does not recognize.
		bumpCoverage(rep, MarkerOrphanEval, "out_of_scope")
	}
	return failures
}

// singlePathRef reports whether ref reads as one path rather than a list: no
// separator and no whitespace. The write-side twin lives in
// mutate.validateSpec's own copy (the keep-each-package's-copy convention
// gen.heldEpicByKey documents); the two must agree or a shape the gate
// refuses could still stat here, or the reverse.
func singlePathRef(ref string) bool {
	return !strings.ContainsAny(ref, "; \t")
}

// What a ref_unreadable detail tells the author to do, by what was being
// checked: a test binding's ref may be a YAML list, a baseline artifact never
// is (artifact-schema.md, The eval model).
const (
	testRefAdvice          = "list several files as a YAML list under ref, one path per item"
	baselineArtifactAdvice = "a baseline artifact names one repo-relative file"
)

// orphanEvalCheckPath stats one path-shaped ref under root: both present and
// missing bump checked, uniform with every other marker in this file, where
// a candidate-producing node (here, criterion) still counts as checked, not
// skipped (Report.Coverage's own contract; finishActiveBasisNode,
// reportAnsweredQuestion, finishStaleSpec, and reportMissingSurface all
// bump checked whether or not the comparison also produced a Candidate).
// Missing additionally appends the ref to find, the candidate the existence
// rule exists for. A hard stat failure, distinct from absence, bumps
// unresolvable instead and is returned as a failure detail rather than
// stopping the rest of the node's criteria, so one unreadable ref never
// swallows a genuine candidate a sibling criterion already found (task-8-
// brief.md's found-and-failed accumulation, mirrored here per the engine
// contract).
func orphanEvalCheckPath(rep *Report, root, criterionID, ref, clock, advice string, find *orphanEvalFinding) (failed bool, detail string) {
	// A ref carrying a separator or whitespace is a list, not a path, and
	// statting the whole string finds nothing by that name: the Acme false
	// positive (JC 2026-08-06), three healthy features reported as drifted
	// over semicolon-joined baseline artifacts. Never-guess applies: the
	// guard refuses to split and stat the pieces (that would bless a shape
	// the spec gate now refuses at write time) and reports not-looked-at
	// instead, the same unresolvable honesty a bare test name gets.
	if !singlePathRef(ref) {
		bumpCoverage(rep, MarkerOrphanEval, "unresolvable")
		return true, fmt.Sprintf("%q is not a single path (criterion %s); %s", ref, criterionID, advice)
	}
	present, err := statPath(root, ref)
	if err != nil {
		bumpCoverage(rep, MarkerOrphanEval, "unresolvable")
		return true, fmt.Sprintf("could not stat %s (criterion %s): %s", ref, criterionID, err)
	}
	bumpCoverage(rep, MarkerOrphanEval, "checked")
	if !present {
		find.missing = append(find.missing, orphanEvalItem{criterionID: criterionID, ref: ref, clock: clock})
	}
	return false, ""
}

// finishOrphanEval records Compared for one feature whose every criterion
// was read without a hard stat failure (a failure on one ref does not stop
// the rest, so Compared and the Uncovered row are decided from the WHOLE
// accumulated outcome, matching finishStaleSpec one marker over), and, when
// find carries any missing ref, appends ONE Candidate naming every one of
// them, never one Candidate per criterion.
func finishOrphanEval(rep *Report, f *store.Feature, find orphanEvalFinding, failures []string) {
	if len(failures) > 0 {
		rep.Uncovered = append(rep.Uncovered, Uncovered{
			Marker: MarkerOrphanEval, Noun: store.NounFeature, ID: f.ID, Key: nodeKey(f),
			Reason: "ref_unreadable", Detail: strings.Join(failures, "; "),
		})
	} else {
		markCompared(rep, store.NounFeature, f.ID)
	}
	if find.empty() {
		return
	}
	rep.Candidates = append(rep.Candidates, Candidate{
		Marker: MarkerOrphanEval,
		Noun:   store.NounFeature,
		ID:     f.ID,
		Key:    nodeKey(f),
		Detail: orphanEvalDetail(find),
		Files:  orphanEvalFiles(find),
		Since:  orphanEvalSince(f, find),
	})
}

// orphanEvalDetail renders one human sentence naming every missing ref, each
// labeled by its own criterion id so a reader can tell which criterion it
// belongs to without re-opening the eval file.
func orphanEvalDetail(find orphanEvalFinding) string {
	parts := make([]string, len(find.missing))
	for i, m := range find.missing {
		parts[i] = fmt.Sprintf("%s (%s)", m.ref, m.criterionID)
	}
	return fmt.Sprintf("%s no longer exists", strings.Join(parts, ", "))
}

// orphanEvalFiles is find's missing refs alone, the repo-relative paths
// Candidate.Files carries.
func orphanEvalFiles(find orphanEvalFinding) []string {
	out := make([]string, len(find.missing))
	for i, m := range find.missing {
		out[i] = m.ref
	}
	return out
}

// orphanEvalSince anchors a candidate's since-when to whichever clock its
// missing refs came from: accepted_against's commit for a preserve-only
// finding, verified_against's for an introduce-only one. A finding that
// mixes both anchors to accepted_against unconditionally: approval always
// precedes a done crossing in the lifecycle (a feature cannot reach done
// without having been approved first), so accepted_against's commit is
// always the earlier of the two whenever both exist, and the earliest
// anchor is what bounds the whole diff a human would read. Either leg's own
// zero anchor (a pre-Task-2 record) reads as nil, the same "nothing to date it
// by, not a failed check" rule basisSince and staleSpecSince both already give
// one marker over.
func orphanEvalSince(f *store.Feature, find orphanEvalFinding) *store.RevisionRef {
	var sawPreserve, sawIntroduce bool
	for _, m := range find.missing {
		switch m.clock {
		case store.PolarityPreserve:
			sawPreserve = true
		case store.PolarityIntroduce:
			sawIntroduce = true
		}
	}
	switch {
	case sawPreserve:
		if f.AcceptedAgainst.Commit.Value == "" {
			return nil
		}
		c := f.AcceptedAgainst.Commit
		return &c
	case sawIntroduce:
		if f.VerifiedAgainst.Commit.Value == "" {
			return nil
		}
		c := f.VerifiedAgainst.Commit
		return &c
	default:
		return nil
	}
}

// orphanEvalUncovered appends one Uncovered row for a feature whose eval
// file itself could not be turned into a checkable contract at all: no key
// to resolve a path from, absent despite HasEval, malformed YAML, or a
// well-formed document ValidateContract refuses. None of the feature's
// criteria are visited in any of these cases, so no Coverage bucket is
// bumped either: Coverage counts criteria for this marker (documented at
// Report.Coverage), and there is no criterion count to attribute a whole-
// file failure to; the Uncovered row alone is what keeps this from reading
// as silence (artifact-schema.md: "a clean answer asserts coverage").
func orphanEvalUncovered(rep *Report, f *store.Feature, detail string) {
	rep.Uncovered = append(rep.Uncovered, Uncovered{
		Marker: MarkerOrphanEval, Noun: store.NounFeature, ID: f.ID, Key: nodeKey(f),
		Reason: "eval_unreadable", Detail: detail,
	})
}

// orphanEvalForNode is ForNode's orphan_eval entry: a feature carrying an
// eval file whose polarity clock has started for at least one criterion, not
// cancelled, reusing reportOrphanEval verbatim, the sweep's own per-feature
// body above (sweepOrphanEval). The cancelled check folds in directly rather
// than the sweep's own separate exempt-and-count arm, for the identical
// reason missingSurfaceForNode already gives one marker up: ForNode has no
// per-node Exempted count for the distinction to attach to. Every other noun
// (an epic, a task, a question) carries no eval file of its own, so the type
// assertion alone already excludes them, matching sweepOrphanEval's own
// features-only loop.
func orphanEvalForNode(t *store.Tree, rep *Report, n store.Node) {
	f, ok := n.(*store.Feature)
	if !ok || !f.HasEval {
		return
	}
	inScope := !acceptedAgainstEmpty(f.AcceptedAgainst) || !verifiedAgainstEmpty(f.VerifiedAgainst)
	if !inScope || f.Status == "cancelled" {
		return
	}
	reportOrphanEval(t, rep, f)
}
