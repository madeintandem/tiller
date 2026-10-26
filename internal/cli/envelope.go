package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file holds the stable JSON output contract every --json command shares
// (cli-surface.md, "Output and exit codes"), plus the small plumbing that maps a
// typed core error onto a process exit code. The envelope shapes are the single
// source later tasks reuse, so keep them additive: a new command carries its
// payload in Data, and a new error reason is a new constant, not a new shape.

// Envelope is the stable JSON shape printed under --json. A successful read or
// mutation sets OK true and Data; a refused command sets OK false and either an
// Error block (a tool-level failure: bad address, not a repo) or a Data block
// alone (a normal negative business answer, like an unready project, that still
// carries structured detail). Exactly one of Data or Error is populated.
type Envelope struct {
	OK      bool       `json:"ok"`
	Command string     `json:"command"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
}

// ErrorBody is the error block: the exit Code the command maps to, a stable
// Reason discriminator the agent loop branches on, a human Message, and the
// optional Candidates an ambiguous-key resolution carries so the caller can
// disambiguate by id. Later tasks extend this additively (current state and
// legal next moves for transition refusals) without changing these fields.
type ErrorBody struct {
	Code          int            `json:"code"`
	Reason        string         `json:"reason"`
	Message       string         `json:"message"`
	Candidates    []string       `json:"candidates,omitempty"`
	PartialResult *upgradeResult `json:"partial_result,omitempty"`
	Noun          string         `json:"noun,omitempty"`
	Node          string         `json:"node,omitempty"`
	Task          string         `json:"task,omitempty"`
	Ref           string         `json:"ref,omitempty"`
	Prerequisite  string         `json:"prerequisite,omitempty"`
}

// Reason discriminators the CLI sets that do not originate in a store.EnvError
// (which carries its own store.Reason* value). These name the resolver's typed
// errors and the generic fallback.
const (
	reasonNotFound          = "not_found"
	reasonNounMismatch      = "noun_mismatch"
	reasonAmbiguousKey      = "ambiguous_key"
	reasonError             = "error"
	reasonReviewGrainLocked = "review_grain_locked"
)

// WriteSuccess writes a success envelope (OK true) carrying data.
func WriteSuccess(w io.Writer, command string, data any) error {
	return WriteResult(w, command, true, data)
}

// WriteResult writes a result envelope with an explicit ok flag. It serves both
// success (ok true) and the exit-1 negative business reports that still carry
// structured Data and no Error block (ok false).
func WriteResult(w io.Writer, command string, ok bool, data any) error {
	return writeEnvelope(w, Envelope{OK: ok, Command: command, Data: data})
}

// WriteError writes an error envelope derived from a typed error and returns the
// process exit code the error maps to (cli-surface.md exit scheme).
func WriteError(w io.Writer, command string, err error) (int, error) {
	body := classify(err)
	werr := writeEnvelope(w, Envelope{OK: false, Command: command, Error: body})
	return body.Code, werr
}

// writeEnvelope marshals e as indented JSON with a trailing newline so the
// output is both human-skimmable and golden-file stable.
func writeEnvelope(w io.Writer, e Envelope) error {
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// classify maps a typed core error onto an ErrorBody and its exit code. The
// store's EnvError carries its own code and reason; a top-level error that
// answers its own ExitCode()/Reason() speaks for itself and wins next; the
// resolver's typed errors are exit-2 misuse with stable reasons for anything
// left bare; anything else is treated as misuse (exit 2), matching the
// cobra-level fallback in Run.
func classify(err error) *ErrorBody {
	var envErr *store.EnvError
	if errors.As(err, &envErr) {
		return &ErrorBody{Code: envErr.ExitCode(), Reason: envErr.Reason, Message: envErr.Error()}
	}
	var recovery *mutate.RecoverReviewError
	if errors.As(err, &recovery) {
		return &ErrorBody{Code: recovery.ExitCode(), Reason: recovery.Reason, Message: recovery.Error(), Noun: recovery.Noun, Node: recovery.NodeID, Task: recovery.TaskID, Ref: recovery.Ref, Prerequisite: recovery.Prerequisite}
	}

	// mutate.ReviewGrainLockedError deliberately implements neither ExitCoder
	// nor Reason() (its own doc comment: "the report sets that"), because
	// configure's own path (reportConfigureError, configure.go) intercepts it
	// directly and renders it as a BLOCKED business answer, exit 1, a data
	// block, reason "review_grain_locked". The approve/transition path never
	// added a matching arm to transitionBlockedView, so this same error
	// surfacing from a re-approve (transitionEpic's or transitionFeature's own
	// Changeability guard)
	// fell through every branch below to the generic misuse fallback, reason
	// "error", exit 2. This checks for it ahead of that fallback so the
	// reason matches configure's, without adopting configure's exit code or
	// data-block shape: the human message already named the refusal
	// correctly (Error() itself says "review surface"), so only the machine
	// reason was ever wrong.
	var locked *mutate.ReviewGrainLockedError
	if errors.As(err, &locked) {
		return &ErrorBody{Code: 2, Reason: reasonReviewGrainLocked, Message: err.Error()}
	}
	var grainBlocked *mutate.ReviewGrainChangeBlockedError
	if errors.As(err, &grainBlocked) {
		return &ErrorBody{Code: 2, Reason: grainBlocked.Reason, Message: err.Error()}
	}

	// A top-level error that implements ExitCoder is checked before the three
	// concrete resolve.* branches below, not after, because one now can wrap a
	// resolve error and still be judged here: mutate's parentUnknownErr
	// (add-feature staged under an epic neither live nor staged earlier on
	// this amendment) carries the live resolver's *resolve.ErrNotFound through
	// Unwrap so a Go-level errors.As still finds it, while its own Error()
	// names a second possibility bare ErrNotFound never could. Checked first,
	// that fuller message and its own Reason() reach the envelope; checked
	// after (the order this branch used to run in), errors.As on *resolve.ErrNotFound
	// below would unwrap straight past it and print only the live-tree half.
	// A plain, unwrapped resolve.* error is untouched: none of the three
	// implements ExitCoder itself, so this branch never matches one directly.
	var coder store.ExitCoder
	if errors.As(err, &coder) {
		reason := reasonError
		var r reasoner
		if errors.As(err, &r) {
			reason = r.Reason()
		}
		return &ErrorBody{Code: coder.ExitCode(), Reason: reason, Message: err.Error()}
	}

	var amb *resolve.ErrAmbiguousKey
	if errors.As(err, &amb) {
		cands := make([]string, len(amb.Candidates))
		for i, c := range amb.Candidates {
			cands[i] = c.NodeID()
		}
		return &ErrorBody{Code: 2, Reason: reasonAmbiguousKey, Message: amb.Error(), Candidates: cands}
	}

	var nm *resolve.ErrNounMismatch
	if errors.As(err, &nm) {
		return &ErrorBody{Code: 2, Reason: reasonNounMismatch, Message: nm.Error()}
	}

	var nf *resolve.ErrNotFound
	if errors.As(err, &nf) {
		return &ErrorBody{Code: 2, Reason: reasonNotFound, Message: nf.Error()}
	}

	return &ErrorBody{Code: 2, Reason: reasonError, Message: err.Error()}
}

// reasoner is implemented by cli-local typed errors that carry a stable reason
// discriminator (for example a usage error), so classify can surface it.
type reasoner interface {
	Reason() string
}

// usageError is a cli-level misuse (exit 2): a malformed flag value the command
// rejects before doing any work. It carries a stable reason for the envelope.
type usageError struct {
	reason string
	msg    string
}

func (e *usageError) Error() string  { return e.msg }
func (e *usageError) ExitCode() int  { return 2 }
func (e *usageError) Reason() string { return e.reason }

// exitError carries a process exit code out of a command's RunE without making
// Run re-print anything: the command has already emitted its envelope. Run maps
// it to the process exit status; any other error from cobra is misuse (exit 2).
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }
func (e *exitError) ExitCode() int { return e.code }

// jsonOut reports whether the caller asked for machine-readable JSON via the
// persistent --json flag wired on the root command.
func jsonOut(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// emitSuccess writes a command's success outcome: a success envelope in JSON
// mode, or the human summary line otherwise. It is emitReport's ok-true, exit-0
// shorthand, so a mutating command's success carries the same acting-contributor
// attribution emitReport does.
func emitSuccess(cmd *cobra.Command, command string, data any, human string) error {
	return emitReport(cmd, command, true, data, human, 0)
}

// emitFailure writes a command's tool-level failure (error envelope to stdout in
// JSON mode, a stderr line otherwise) and returns an exitError carrying the
// mapped exit code so Run reports it without re-printing.
func emitFailure(cmd *cobra.Command, command string, err error) error {
	if jsonOut(cmd) {
		code, _ := WriteError(cmd.OutOrStdout(), command, err)
		return &exitError{code: code, msg: err.Error()}
	}
	body := classify(err)
	fmt.Fprintln(cmd.ErrOrStderr(), "tiller:", err.Error())
	return &exitError{code: body.Code, msg: err.Error()}
}

// emitReport writes a command's structured result with an explicit ok flag and
// exit code: it is the path for a normal negative business answer (an unready
// project, exit 1) that still carries a Data block rather than an Error block. A
// not-ok report returns an exitError carrying code; an ok report returns nil.
//
// Every mutating command's report additionally carries the acting-contributor
// attribution (design doc, "The fallback speaks"): resolveActor decides this
// from cmd alone (a read command, carrying no --contributor flag, resolves to the
// zero actor and gets neither addition), so this stays the single place that
// rule is enforced for both emitReport's direct callers and emitSuccess,
// which now delegates here.
func emitReport(cmd *cobra.Command, command string, ok bool, data any, human string, code int) error {
	return emitAttributed(cmd, command, ok, data, human, code, "")
}

// emitAttributed is emitReport and emitReportWithReadout's shared write path.
// block is the gate readout text emitReportWithReadout appends after the
// primary confirmation ("" for every other caller, where the primary
// confirmation is the whole report). Attribution is stitched in here, once,
// ahead of the block, so a mutating command's report reads as attribution
// then a following section rather than attribution trailing after a page of
// readout (the same placement humanTransition already gives submit's own
// unpushedReviewWarning: appended to the primary confirmation, ahead of
// anything that follows it):
//
//	[json] a top-level additive "actor" field on the data block, the same
//	map-merge mechanism withReadout uses for "readout" (withActor).
//	[human] the "acting as <id> (<source>)" line appended to the primary
//	confirmation when the winning source is a fallback (actorEcho).
func emitAttributed(cmd *cobra.Command, command string, ok bool, data any, human string, code int, block string) error {
	a := resolveActor(cmd)
	data = withActor(data, a)
	if echo := actorEcho(a); echo != "" {
		human = appendLine(human, echo)
	}
	if block != "" {
		if jsonOut(cmd) {
			data = withReadout(data, block)
		} else {
			human = human + "\n\n" + block
		}
	}
	return writeReport(cmd, command, ok, data, human, code)
}

// writeReport is the raw envelope-and-human writer: the shared tail every
// emitReport-family call ends on, once attribution and any readout block are
// already stitched into data and human.
func writeReport(cmd *cobra.Command, command string, ok bool, data any, human string, code int) error {
	if jsonOut(cmd) {
		_ = WriteResult(cmd.OutOrStdout(), command, ok, data)
	} else {
		w := cmd.OutOrStdout()
		if !ok {
			w = cmd.ErrOrStderr()
		}
		if human != "" {
			// Success output renders for a terminal reader; piped and stderr
			// output stay the raw wire format (see tty.go).
			if ok {
				human = renderHumanTTY(human)
			}
			fmt.Fprintln(w, human)
		}
	}
	if !ok {
		return &exitError{code: code, msg: human}
	}
	return nil
}

// emitReportWithReadout is emitReport plus the moment's readout block,
// design doc's "Command surface": "gates emit it themselves ... There is
// nothing for an agent to forget, and a developer running the command
// directly gets the same brief." emitReport keeps its own signature and every
// existing caller is unaffected; this is the gate commands' own tail
// (readout_gate.go, which is the authoritative set), called only when m names
// one of them. act is the gate's
// own "what just happened" payload (views.GateAct), built by the call site
// from data already in scope there (the resolved node, the mutation
// result) and threaded through to renderReadoutFor so the Verification
// section can prove the act rather than describe the tree a second time
// (a gate call site already has the node
// and the result in scope, runAttest's n/res.TargetID/res.TargetKind named
// in the finding, so the plumbing carries them rather than a content
// function falling back to a tree-wide tally). nil for a gate content
// function has nothing act-specific to say (none of them today), though
// the parameter is not itself optional in spirit: every gate call site
// populates it.
//
// The block follows the existing confirmation rather than replacing it: on a
// batch approve those lines are the per-node record of what transitioned and
// what was already in the target state, and a summary cannot stand in for
// them.
// A not-ok report (a blocked business answer) carries no readout at all: the
// gate did not complete, so there is nothing new for a fresh tree read to
// say that the existing blocked envelope has not already said.
func emitReportWithReadout(cmd *cobra.Command, command string, ok bool, data any, human string, code int, m views.Moment, root string, act *views.GateAct) error {
	if !ok {
		return emitReport(cmd, command, ok, data, human, code)
	}
	// The acting contributor is the reader the block's ranked Next list is
	// ranked for, and every gate that reaches here carries --contributor, so
	// this resolves the same identity the mutation just attributed to.
	block, err := renderReadoutFor(root, m, act, resolveActingContributor(cmd, root))
	if err != nil {
		// The gate itself already succeeded (ok is true here); a reporting
		// problem on top of that (the tree fails to reload right after its own
		// write landed) must not turn into a transition failure by way of this
		// wrapper. Logged to stderr and the readout omitted, exactly the
		// brief's own rule, rather than silently dropped where nothing would
		// ever surface a real regression here. Covered by
		// TestEmitReportWithReadoutOmitsTheBlockOnALoadFailure
		// (readout_gate_test.go): an earlier review found this path right but
		// unguarded, a throwaway test written, confirmed, then deleted.
		fmt.Fprintf(cmd.ErrOrStderr(), "tiller: readout for %s not available: %v\n", command, err)
		return emitReport(cmd, command, ok, data, human, code)
	}
	return emitAttributed(cmd, command, ok, data, human, code, block)
}

// withReadout marshals data to a map and sets "readout" to block, so an
// existing typed payload (TransitionView, BatchData, questionView,
// AttestView, syncView, ...) keeps every field it already carries and the
// block rides additively (design doc: "--json carries the block additively.
// ... No existing payload shape changes."). A marshal failure (none of the
// payload shapes in this package can produce one; this is the same defensive
// fallback batchEntry (batch.go) already takes for the identical
// marshal-then-remarshal move) returns data unchanged rather than losing the
// caller's real payload over a field that could not be added.
//
// The map round trip passes every numeric field through Go's untyped JSON
// number (float64): TransitionView.PR *int, say, comes back out of
// json.Unmarshal as a float64 wrapped in the map, and json.Marshal-ing that
// back to text renders it as encoding/json's own shortest-round-trip
// decimal, which for a whole number carries no fractional part or exponent
// ("41", never "41.0" or "4.1e1") at any value a PR number or a similar
// small integer in this codebase actually takes. Verified directly (a
// throwaway program marshaling a *int through this exact path) rather than
// assumed; flagged here because it went unnoticed in review and
// is the kind of thing worth a reader knowing rather than rediscovering.
func withReadout(data any, block string) any {
	b, err := json.Marshal(data)
	if err != nil {
		return data
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		return data
	}
	m["readout"] = block
	return m
}

// withNext is withReadout's sibling for the "next" field: the same map merge,
// so a completing command's payload keeps every field it already carries and
// the ranked list rides beside them. A marshal failure returns data unchanged.
func withNext(data any, next views.Next) any {
	b, err := json.Marshal(data)
	if err != nil {
		return data
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		return data
	}
	m["next"] = next
	return m
}

// actor is the acting-contributor attribution a mutating command's report carries
// (design doc, "The fallback speaks"): who resolveActingContributor picked, and
// which of the three sources (contributorSourceFlag, contributorSourceEnv,
// contributorSourceIdentity; add.go) actually won.
type actor struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

// mutatingCommand distinguishes actor-bearing commands from read filters:
// ls also has --contributor, but uses it to filter ancestor contributions.
// Only commands wired through addContributorFlag carry actor attribution.
func mutatingCommand(cmd *cobra.Command) bool {
	return cmd.Annotations["acting-contributor"] == "true"
}

// resolveActor resolves cmd's acting-contributor attribution, or the zero actor for
// a read command (mutatingCommand false) or an unresolvable repo root. The
// latter is not observed in practice: every call site that reaches here is
// already past loadTree, which resolves the identical root successfully.
func resolveActor(cmd *cobra.Command) actor {
	if !mutatingCommand(cmd) {
		return actor{}
	}
	root, err := repoRoot()
	if err != nil {
		return actor{}
	}
	id, source := resolveActingContributorWithSource(cmd, root)
	if id == "" {
		return actor{}
	}
	return actor{ID: id, Source: source}
}

// actorEcho renders the human "the fallback speaks" line, or "" when there is
// nothing to say: no actor resolved, or the winning source was the explicit
// --contributor flag, which is already on the command line the user typed.
func actorEcho(a actor) string {
	if a.ID == "" || a.Source == contributorSourceFlag {
		return ""
	}
	return fmt.Sprintf("acting as %s (%s)", a.ID, a.Source)
}

// withActor is withReadout's sibling for the "actor" field: the same
// map-merge mechanism, so a mutating command's JSON payload keeps every field
// it already carries and "actor" rides along additively, always present
// (design doc: "ALWAYS present regardless of source"). The zero actor (a read
// command, or a repo root that failed to resolve) leaves data untouched.
func withActor(data any, a actor) any {
	if a.ID == "" {
		return data
	}
	b, err := json.Marshal(data)
	if err != nil {
		return data
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		return data
	}
	m["actor"] = a
	return m
}

// appendLine appends s as a new line onto human, guarding the case where
// human is empty (no observed caller today, kept honest rather than assumed).
func appendLine(human, s string) string {
	if human == "" {
		return s
	}
	return human + "\n" + s
}
