package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/readiness"
	"github.com/madeintandem/tiller/internal/core/review"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file wires `tiller readout <seam>` (docs/spec/cli-surface.md),
// the command a harness runs at a seam boundary to get the one block it
// reproduces verbatim in its reply, and a developer runs themselves to read
// the same thing in a terminal. It is a thin caller: every fact views cannot
// derive without I/O (the git commit and dirtiness, `check readiness`, each
// feature's parsed feature.eval.md, each epic's spec-and-eval status) is read
// here and handed to views.ReadoutOfInput, which does the actual derivation.
// renderReadout (readout_render.go) turns the result into the markdown text
// that becomes both the human line and the --json "readout" field, one
// rendering for both audiences.
//
// renderReadoutOf and renderReadoutFor, below runReadout, are that same
// gathering-and-rendering path factored for a second caller: the four human
// gates (readout_gate.go) render their own block after their existing
// confirmation, through emitReportWithReadout (envelope.go), rather than
// through this command. Task 5 left the logic inline in runReadout because
// it had only the one caller; task 6 is the second caller
// anticipated ("reuse that helper rather than writing a second copy"), so
// this is that factoring.

// readoutSeams registers skill seam readouts. Human gates emit their own
// readout after the mutation instead of entering through this command.
var readoutSeams = map[string]views.Moment{
	"orient":          views.MomentOrient,
	"derive-baseline": views.MomentDeriveBaseline,
	"decompose":       views.MomentDecompose,
	"specify":         views.MomentSpecify,
	"execute":         views.MomentExecute,
	"amend":           views.MomentAmend,
}

// readoutSeamOrder fixes the seam list's order in the unknown-seam refusal, so
// two runs report the same text (a map's range order is unspecified).
var readoutSeamOrder = []string{"orient", "derive-baseline", "decompose", "specify", "execute", "amend"}

// readoutData is the --json "data" block: the moment named back (an agent
// dispatching readout from a script can confirm it got the seam it asked
// for), the since sha when the correction block was asked for, and the
// rendered block under the "readout" key the brief specifies verbatim.
type readoutData struct {
	Moment  string `json:"moment"`
	Since   string `json:"since,omitempty"`
	Readout string `json:"readout"`
}

func newReadoutCmd() *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use:   "readout <seam>",
		Short: "Print the one consistent readout block for a seam.",
		Long: "readout derives and prints the block a harness reproduces verbatim at a\n" +
			"seam boundary: what happened, what was verified, and what needs you. The\n" +
			"seam is one of orient, derive-baseline, decompose, specify, execute, or amend. The\n" +
			"terminal text and the --json \"readout\" field are the same string, so the\n" +
			"developer and the harness never see two different accounts of one run.\n\n" +
			"--since <sha> prints the lighter correction block instead: what the event\n" +
			"stream recorded since that commit, the one the last block reported, plus\n" +
			"the queue line and the ranked Next list. It is the block a correction or\n" +
			"other small action inside a gate closes its reply with; a seam exit keeps\n" +
			"the full one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReadout(cmd, args[0], since)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "render the correction block: what changed since this commit, the one the last block reported")
	return cmd
}

func runReadout(cmd *cobra.Command, seam, since string) error {
	moment, ok := readoutSeams[seam]
	if !ok {
		return emitFailure(cmd, "readout", &usageError{
			reason: "unknown_seam",
			msg:    fmt.Sprintf("unknown seam %q (want one of %s)", seam, strings.Join(readoutSeamOrder, ", ")),
		})
	}

	// loadTree is the same store-loading, error-mapping path every other read
	// command in this package takes (ls.go's runLs), so a
	// missing manifest or a malformed node file refuses the same way here as
	// everywhere else. This is the one place in the readout* files that must
	// actually refuse on a load failure (exit 2/3): renderReadoutFor below
	// degrades on the identical failure instead, because a human gate that
	// reaches it has already completed its own write and must not be turned
	// into a failure by a reporting problem.
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "readout", err)
	}
	owner := resolveActingContributor(cmd, tree.Root)

	if since != "" {
		rendered, err := stableReadout(tree, func() (string, error) {
			return renderCorrectionOf(tree, moment, since, owner)
		})
		if err != nil {
			return emitFailure(cmd, "readout", err)
		}
		data := readoutData{Moment: string(moment), Since: since, Readout: rendered}
		return emitReport(cmd, "readout", true, data, rendered, 0)
	}

	// The seam path never carries an Act: nothing about `tiller readout
	// <seam>` is the tail of a mutation, so there is no "what just
	// happened" to hand in, only the tree as it now stands.
	rendered, err := stableReadout(tree, func() (string, error) {
		return renderReadoutOf(tree, moment, nil, owner), nil
	})
	if err != nil {
		return emitFailure(cmd, "readout", err)
	}
	data := readoutData{Moment: string(moment), Readout: rendered}
	// readout never itself fails the way check readiness does: it reports what
	// is true, it does not gate on it (that is NeedsYou's and Checks' job
	// inside the block). ok is always true here; exit code 0.
	return emitReport(cmd, "readout", true, data, rendered, 0)
}

// gatherReadoutInput reads every I/O-derived fact ReadoutOfInput cannot read
// itself (views performs no I/O, this file's own opening comment) off an
// already-loaded tree, for moment m. It is renderReadoutOf's own gathering
// step, factored out so a test can inspect the assembled ReadoutInput
// directly rather than only the rendered text that follows it (the covering
// assertion for UnpushedReviews's execute-only gate, below, needs exactly
// that: the rendered block looks the same whether the field is nil or empty,
// so only the input itself can show the gather was skipped).
func gatherReadoutInput(tree *store.Tree, m views.Moment, act *views.GateAct, owner string) views.ReadoutInput {
	// Git facts: read-only through internal/vcs, never derived by views, which
	// performs no I/O (readout.go's own doc comment on ReadoutInput). readout
	// is read-only and informational, ok always true, so a git that cannot be
	// read must degrade the headline rather than refuse the whole command
	// (review finding 2: this used to hard-refuse with exit 3 on a copied
	// tree with no git identity configured). detectIntegrationBranch (init.go)
	// is the precedent: an informational git read degrades silently on
	// failure rather than failing the command it serves.
	commit, dirty := readoutGitFacts(vcs.New(tree.Root))

	// tree.Manifest is already loaded by store.Load; readiness.Check is the same
	// gate `tiller check readiness` runs (check_readiness.go), read straight
	// rather than re-parsing the manifest a second time.
	ready := readiness.Check(tree.Manifest, tree).Ready

	// UnpushedReviews is gathered only for execute: it is the sole moment
	// executeContent (internal/views/readout.go) ever reads it from, and the
	// gather itself is two git subprocesses per in-review PR-backed task
	// (UnpushedTip's merge-base plus ShortSHA). Every other moment already
	// renders a nil slice the same as "checked, found nothing" (ReadoutInput's
	// own doc comment on UnpushedReviews), so skipping the gather changes no
	// rendered output; it only stops paying for a read no other moment shows.
	var unpushedReviews []views.UnpushedReview
	if m == views.MomentExecute {
		unpushedReviews = readoutUnpushedReviews(tree)
	}

	// These live reads serve only their respective moments, just like the
	// execute-only review reads above.
	var epicSpecs map[string]views.EpicSpecStatus
	if m == views.MomentDecompose {
		epicSpecs = readoutEpicSpecs(tree)
	}
	var amendmentPreconditions map[string][]mutate.PreconditionValidity
	if m == views.MomentAmend {
		amendmentPreconditions = readoutAmendmentPreconditions(tree)
	}

	// The ranked Next list reads execution observed on local branches and
	// worktrees and the reader's recent events on every moment,
	// gates' trailing readouts included; either read degrades to none.
	next := readinput.ForNext(tree, owner, time.Now())
	workflow := next.Workflow
	return views.ReadoutInput{
		Commit:                 commit,
		Dirty:                  dirty,
		Ready:                  ready,
		DesignDocPresent:       readoutDesignDocPresence(tree),
		QueueInput:             views.QueueInput{Workflow: workflow},
		Contracts:              workflow.Contracts,
		Workflow:               workflow,
		EpicSpecs:              epicSpecs,
		AmendmentPreconditions: amendmentPreconditions,
		UnpushedReviews:        unpushedReviews,
		Act:                    act,
		Contributor:            owner,
		Recent:                 next.Recent,
		Now:                    next.Now,
	}
}

// readoutDesignDocPresence supplies live prose checks without teaching views I/O.
func readoutDesignDocPresence(tree *store.Tree) map[string]bool {
	present := make(map[string]bool, len(tree.Baseline.Docs))
	for _, doc := range tree.Baseline.Docs {
		present[doc.Path] = store.BaselineDocPresent(tree.Root, doc.Path)
	}
	return present
}

// renderReadoutOf gathers the I/O-derived facts ReadoutOfInput cannot read
// itself off an already-loaded tree (gatherReadoutInput, above) and renders
// the block. runReadout and renderReadoutFor (below) both call this rather
// than each carrying its own copy of the git/readiness/contracts/epic-specs
// gathering. act is the gates' own "what just happened" payload
// (readout_gate.go), nil for the seam path, so a gate's Verification section
// can prove the act rather than re-describe the tree: views.ReadoutInput.Act
// carries the reasoning in full.
func renderReadoutOf(tree *store.Tree, m views.Moment, act *views.GateAct, owner string) string {
	return renderReadout(views.ReadoutOfInput(tree, m, gatherReadoutInput(tree, m, act, owner)))
}

// renderReadoutFor loads the tree fresh from root and renders m's block: the
// path every human gate calls (envelope.go's emitReportWithReadout), so a
// gate's block and a `tiller readout <seam>` a developer runs right after it
// are the identical rendering with no second copy of the gathering logic.
// It reloads from disk rather
// than taking the caller's in-memory tree because every gate call site has
// only tree.Root in hand by the time it reports success (batch.go's
// applyTransitionBatch), and a fresh read is
// exactly what a `tiller readout <seam>` run as a separate command right
// after the gate would see, so the two can never diverge. act carries the
// gate's own mutation result (readout_gate.go builds it at each gate call
// site); the seam path always passes nil through runReadout above.
//
// A load failure here is returned rather than degraded, unlike the git and
// readiness reads renderReadoutOf takes: the caller (emitReportWithReadout)
// is the one that decides to log and omit rather than fail the command, per
// the brief's own rule that a reporting problem must never become a
// transition failure; this function stays a plain error-returning read so
// that rule lives in exactly one place.
func renderReadoutFor(root string, m views.Moment, act *views.GateAct, owner string) (string, error) {
	tree, err := store.Load(root)
	if err != nil {
		return "", err
	}
	return stableReadout(tree, func() (string, error) {
		return renderReadoutOf(tree, m, act, owner), nil
	})
}

// renderCorrectionOf renders the correction block for
// `tiller readout <seam> --since <sha>`: it validates the sha, reads the
// event files git names as added since it, reads the same git facts the
// full block reads, and hands all of it to views.CorrectionOfInput. Unlike
// renderReadoutOf, a failure here is returned rather than degraded: --since
// is an explicit ask about git history, so a sha git cannot place is a
// misuse (exit 2), and an event file that cannot be read is an environment
// fault, never something to leave silently out of Changed.
func renderCorrectionOf(tree *store.Tree, m views.Moment, since, owner string) (string, error) {
	g := vcs.New(tree.Root)
	short, err := resolveSince(g, since)
	if err != nil {
		return "", err
	}
	evs, err := readoutEventsSince(tree, g, since)
	if err != nil {
		return "", err
	}
	commit, dirty := readoutGitFacts(g)
	next := readinput.ForNext(tree, owner, time.Now())
	return renderCorrection(views.CorrectionOfInput(tree, m, views.CorrectionInput{
		Commit:           commit,
		Dirty:            dirty,
		Contributor:      owner,
		Since:            short,
		Events:           evs,
		DesignDocPresent: readoutDesignDocPresence(tree),
		Workflow:         next.Workflow,
		Recent:           next.Recent,
		Now:              next.Now,
	})), nil
}

// resolveSince validates --since and returns the commit's short sha for the
// headline. The flag expects the commit the last readout block reported
// ("committed at `<sha>`"), so it must name a commit git can resolve that
// sits in HEAD's history; either failure is a misuse, exit 2, naming the
// flag and what it expects. A git that cannot answer at all keeps its own
// environment error (exit 3).
func resolveSince(g *vcs.Git, since string) (string, error) {
	// A dash-led value would read as an option to git rev-parse and fail as
	// an environment error; it is a misuse of the flag, so refuse it here
	// with the same reason an unknown sha gets.
	if strings.HasPrefix(since, "-") {
		return "", &usageError{
			reason: "since_unresolvable",
			msg: fmt.Sprintf("--since %q does not name a commit in this repository; "+
				"it expects the commit the last readout block reported (\"committed at `<sha>`\")", since),
		}
	}
	exists, err := g.RefExists(since + "^{commit}")
	if err != nil {
		return "", err
	}
	if !exists {
		return "", &usageError{
			reason: "since_unresolvable",
			msg: fmt.Sprintf("--since %q does not name a commit in this repository; "+
				"it expects the commit the last readout block reported (\"committed at `<sha>`\")", since),
		}
	}
	ancestor, err := g.IsMergedInto(since, "HEAD")
	if err != nil {
		return "", err
	}
	if !ancestor {
		return "", &usageError{
			reason: "since_not_ancestor",
			msg: fmt.Sprintf("--since %q is not an ancestor of HEAD; "+
				"it expects the commit the last readout block reported (\"committed at `<sha>`\"), which this checkout's history must contain", since),
		}
	}
	return g.ShortSHA(since)
}

// readoutEventsSince reads every event file committed since the sha: the
// stream is append-only, one immutable file per event, so the files git
// names as added under .tiller/events between the sha and HEAD are exactly
// the mutations the correction recorded. Order is left to views, which sorts
// by the stream's own rule. An event file that will not parse refuses the
// block rather than dropping the event: Changed is the block's whole reason
// to exist, and a hole in it would read as "nothing happened".
func readoutEventsSince(tree *store.Tree, g *vcs.Git, since string) ([]events.Event, error) {
	rels, err := g.AddedSince(since, ".tiller/events")
	if err != nil {
		return nil, err
	}
	evs := make([]events.Event, 0, len(rels))
	for _, rel := range rels {
		if !strings.HasSuffix(rel, ".yml") {
			continue
		}
		ev, err := events.Load(tree.Root, filepath.FromSlash(rel))
		if err != nil {
			return nil, &store.EnvError{Code: 3, Reason: "event_unreadable", Msg: err.Error()}
		}
		evs = append(evs, ev)
	}
	return evs, nil
}

func stableReadout(tree *store.Tree, project func() (string, error)) (string, error) {
	out, projectionErr := project()
	if err := tree.CheckStable(); err != nil {
		return "", err
	}
	return out, projectionErr
}

// readoutGitFacts reads HEAD and dirtiness for the headline's artifact clause,
// degrading to a zero value on either read's failure rather than propagating
// the error: git missing, the directory not a repo, or an unborn HEAD (no
// commits yet) all mean "no commit to name", which ReadoutOfInput already
// renders honestly (Artifact stays empty when Commit is ""), not a reason to
// refuse a read-only, informational command. status, ls, and check readiness
// all answer with no git at all; readout keeps that company.
func readoutGitFacts(rev vcs.Revisions) (commit string, dirty bool) {
	if c, err := rev.Head(); err == nil {
		commit = c
	}
	if d, err := rev.IsDirty(); err == nil {
		dirty = d
	}
	return commit, dirty
}

// readoutEpicSpecs reads every epic's epic.md and answers decompose's per-epic
// spec check, the I/O ReadoutInput.EpicSpecs's own doc comment says views
// cannot do itself. It always returns a non-nil map for the same reason
// readoutContracts does. ProseWritten asks whether anyone has written over the
// `tiller add epic` stub, through mutate.IsProseStub, the same exported
// predicate the console's read model asks of the same file, so the two surfaces
// can never disagree about it.
//
// There is no second answer here anymore. It used to ask whether epic.eval.md
// validated as a contract, and an epic no longer has one: its acceptance bar is
// the sum of its features' contracts, which readoutContracts already reads at
// feature grain (artifact-schema.md, Epics).
func readoutEpicSpecs(t *store.Tree) map[string]views.EpicSpecStatus {
	out := map[string]views.EpicSpecStatus{}
	for id, e := range t.Epics {
		var st views.EpicSpecStatus
		if p, ok := gen.ProsePath(t, e); ok {
			data, err := store.ReadRepositoryFile(t.Root, p)
			// A read failure (missing epic.md) reads the same as an unwritten
			// stub: both mean the prose was never written.
			st.ProseWritten = err == nil && !mutate.IsProseStub(store.NounEpic, string(data))
		}
		out[id] = st
	}
	return out
}

// readoutAmendmentPreconditions reads every proposed amendment's live
// precondition validity, the I/O ReadoutInput.AmendmentPreconditions's own
// doc comment says views cannot do itself:
// mutate.EvaluateAmendmentPreconditions compares a staged precondition's
// expected value against the current working tree, hashing files through git
// for spec_revision and eval_revision (the identical read approve's own
// checkPreconditions performs). It always returns a non-nil map, even when
// the tree carries no proposed amendments, so amendContent's "not evaluated"
// fallback is never mistaken for a real answer of zero (the same
// nil-versus-empty discipline readoutContracts and readoutEpicSpecs follow).
// An amendment whose batch cannot even resolve (a vanished target) is left
// out of the map rather than failing the whole readout, the same honest
// degrade readoutContracts applies to a feature whose eval file fails to
// parse: `tiller readout amend` is a read, and a stuck amendment is exactly
// what a human runs it to see, not a reason to refuse reporting on every
// other one.
func readoutAmendmentPreconditions(t *store.Tree) map[string][]mutate.PreconditionValidity {
	out := map[string][]mutate.PreconditionValidity{}
	for _, a := range t.Amendments {
		if a.Status != store.AmendmentStatusProposed {
			continue
		}
		pv, err := mutate.EvaluateAmendmentPreconditions(t, mutate.Deps{}, a)
		if err != nil {
			continue
		}
		out[a.ID] = pv
	}
	return out
}

// readoutUnpushedReviews reads the review-state warning for every
// in-review task that records a PR, the I/O ReadoutInput.UnpushedReviews's
// own doc comment says views cannot do itself: whether a task's recorded
// feature branch carries a commit its remote-tracking counterpart does not is
// a live git question, review.ForTask's own (internal/core/review, the one
// computation this file, submit's and status's own call sites, and the
// console's node page all share). Only a PR-backed in-review task is ever
// asked at all; a remote-less submit's branch-ref fallback leaves PR nil,
// which review.ForTask already reads as nothing to compare. A task whose
// check cannot answer (no remote counterpart at all, most commonly) is
// simply left out of the result rather than failing the whole readout, the
// same per-item degrade readoutContracts and readoutEpicSpecs already apply.
//
// renderReadoutOf calls this only for MomentExecute, the one moment
// executeContent (internal/views/readout.go) ever reads its answer from; a
// non-execute readout renders the same block from a nil slice, so this stays
// unconditional in its own right and leaves the gate to its one caller.
func readoutUnpushedReviews(t *store.Tree) []views.UnpushedReview {
	g := vcs.New(t.Root)
	var out []views.UnpushedReview
	for id, tk := range t.Tasks {
		if tk.Status != "in_review" || tk.PR == nil {
			continue
		}
		facts := review.ForTask(t, id, g)
		if facts == nil {
			continue
		}
		out = append(out, views.UnpushedReview{Node: id, Commit: facts.Commit, Upstream: facts.Upstream})
	}
	return out
}
