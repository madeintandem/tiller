package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/spf13/cobra"
)

// This file wires the project-level `tiller sync` command (cli-surface.md, "The
// done crossing"; "Inspecting and maintaining"). It is a thin shell: it loads the
// tree, hands a read-only git window to internal/core/mutate.Sync, and renders
// core records done only after a task branch or its explicitly submitted aggregate
// review branch merges into integration. Intermediate aggregate assembly never
// crosses a task to done; observation, lifecycle crossings, policy violations,
// and events all live in core.
//
// There are blocked-business paths here, and they are the whole reason this file
// classifies at all: sync refuses (exit 1, ok:false, a data block and never an
// error block) when the integration branch it resolved is a feature's own branch,
// when it cannot resolve one, when --accept-fast-forward is used against a ref
// that is not the declared integration branch, and when a node named on
// --accept-fast-forward is outside the set that flag may cross. Everything else
// is either a success or a tool failure (an unknown owner, a git that cannot
// run).
//
// --accept-fast-forward is the one thing sync records on something other than an
// observation, so this file renders it apart: an asserted crossing never joins
// the observed count, never truncates, and says in as many words whose word it
// rests on. It is human-only in the strictest class, beside `attest`
// and the merge itself, which is guidance stated in orientation.md and the
// skills rather than a refusal here (cli-surface.md, "The actor model").

// syncRefusalReason values are the stable discriminators the refusal data block
// carries, the token an agent loop branches on.
const (
	reasonIntoIsFeatureBranch = "into_is_feature_branch"
	reasonIntoUnresolved      = "into_unresolved"
	reasonIntegrationMissing  = "integration_ref_missing"
	reasonNotAttestable       = "not_attestable"
	reasonIntoNotIntegration  = "into_not_integration_branch"
)

// basisHumanAttested is the one crossing basis that is an assertion rather than
// an observation, and it is the single discriminator this file splits the
// confirmation on. The value is defined by artifact-schema.md ("Done basis"),
// which is canonical; the core mints it and this restates it to render it, so
// the two must never drift apart. TestSyncAcceptFastForwardJSONCLI pins the token
// end to end against a real crossing rather than against this constant.
const basisHumanAttested = "human_attested"

// intoSourceUnresolved is what the refusal reports as the value's source when
// nothing resolved at all, so `source` is always populated in the envelope.
const intoSourceUnresolved = "unresolved"

// awaitingHumanLimit caps how many awaiting nodes the human readout names before
// it summarizes the rest. The JSON block carries all of them.
const awaitingHumanLimit = 5

// SyncView is the SUCCESS payload of sync: the recorded done crossings, the
// integration branch they were observed against, the nodes still sitting in
// review after the walk, and the features the human asserted over.
// into_source names where the integration branch came from ("--into",
// "git.integration_branch", or "HEAD" for the branch the checkout is standing
// on). It is on the success envelope, not only the refusal, because a project
// with no manifest field is silently observing whatever branch it happens to be
// on, and that is the one thing a reader needs in order to notice.
//
// attested carries the feature ids the human named on --accept-fast-forward,
// deduped and in the order named. It records the INPUT to the assertion rather
// than its effect, which is why it is not derivable from crossings: at task
// grain the named feature may not itself cross, and an auditor still needs to
// see what was asserted. Which crossings the assertion produced is read off
// each crossing's own basis, the single discriminator; no derived boolean is
// added beside it.
// reopened carries the other direction, and is a separate array rather than a
// crossing with a different status because every entry in crossings means "this
// reached done" and a caller must not have to read a status field to tell the two
// apart. It is absent from the envelope on the ordinary sync that reopened
// nothing.
// skipped is the third outcome, present only when a node earned its rollup and
// sync could not record it.
// checks_due carries the features whose rollup is parked on an unchecked
// feature gate; the run names them so crossing a feature's last task never
// reads as finished while the feature strands.
type SyncView struct {
	Crossings  []CrossingView `json:"crossings"`
	Reopened   []ReopenedView `json:"reopened,omitempty"`
	Skipped    []SkippedView  `json:"skipped,omitempty"`
	Into       string         `json:"into,omitempty"`
	IntoSource string         `json:"into_source,omitempty"`
	Awaiting   []AwaitingView `json:"awaiting,omitempty"`
	Attested   []string       `json:"attested,omitempty"`
	ChecksDue  []CheckDueView `json:"checks_due,omitempty"`
}

// CheckDueView is one feature left awaiting its feature check after the walk.
type CheckDueView struct {
	Noun string `json:"noun"`
	ID   string `json:"id"`
	Key  string `json:"key,omitempty"`
}

// SkippedView is one node whose rollup was earned but whose completion record
// could not be built, so it did not cross. path names the repo-relative path git
// could not resolve, which is both the cause and the fix: a completion record
// pins the node's prose blob and one fingerprint per surface path (a git_tree
// for a directory, a git_blob for a file), and a path resolving to nothing has
// no revision to pin.
//
// It is emitted so a crossing that did not happen is never indistinguishable
// from a clean run (cli-surface.md, "The done crossing"). Nothing else would
// say: the work beneath the node really is complete, so every other surface
// shows a node that simply sits there.
type SkippedView struct {
	Noun string `json:"noun"`
	ID   string `json:"id"`
	Key  string `json:"key,omitempty"`
	Path string `json:"path"`
	// Surface is true when path is a recorded surface path; then Remedy is the
	// unlink that drops it once the code's current location is linked.
	Surface bool   `json:"surface,omitempty"`
	Remedy  string `json:"remedy,omitempty"`
}

// ReopenedView is one rollup-backed node this sync returned to work because new
// work landed beneath it: a done feature that gained a task still to do goes back
// to active, a done epic that gained a feature still to do goes back to ready
// (artifact-schema.md, "History is append-only"). There is no basis field: a
// reverse rests on the tree's own shape rather than on a git observation.
type ReopenedView struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Status string `json:"status"`
}

// CrossingView is one recorded crossing in the sync envelope: the node it crossed
// to done, the observed integration tip, the evidence the crossing rested on, and
// any required gates that were not green at the merge (mirrored into the node's
// policy_violations). Basis is always populated ("ancestry" when the branch is
// contained in the integration branch, "content" when its net change was matched
// by patch-id, "rollup" when the node's non-cancelled children are all done, and
// "human_attested" when a human asserted the crossing on --accept-fast-forward);
// matched_commit names the integration-branch commit whose patch matched and
// appears on a content crossing only. The enum is artifact-schema.md's, "Done
// basis", and the node's own done_basis carries the same token.
//
// Which token each grain records, enumerated rather than generalized:
//
//	task:    ancestry, content, or human_attested
//	feature: rollup or human_attested
//	epic:    rollup
//
// The code that decides each is syncTasks, featureBasis, and crossEpicDone in
// internal/core/mutate/sync.go. Read those for the rules; this list is the shape
// a consumer of the envelope can rely on.
type CrossingView struct {
	Noun                       string                      `json:"noun"`
	ID                         string                      `json:"id"`
	Key                        string                      `json:"key,omitempty"`
	Status                     string                      `json:"status"`
	Observed                   string                      `json:"observed,omitempty"`
	Basis                      string                      `json:"basis"`
	MatchedCommit              string                      `json:"matched_commit,omitempty"`
	PolicyViolations           []string                    `json:"policy_violations,omitempty"`
	HistoricalPolicyViolations []views.PolicyViolationView `json:"historical_policy_violations,omitempty"`
	TouchedDesign              []string                    `json:"touched_design,omitempty"`
	EvalContractMismatch       *EvalContractMismatchView   `json:"eval_contract_mismatch,omitempty"`
}

// EvalContractMismatchView rides a feature crossing whose completion record
// pins an integration eval blob other than accepted_against.eval. accepted is
// the contract check and submit judged in the working tree; verified is the
// integration commit's blob now in verified_against.eval. It is advisory: the
// crossing happened and nothing extra was persisted (cli-surface.md, sync).
type EvalContractMismatchView struct {
	Accepted string `json:"accepted"`
	Verified string `json:"verified"`
}

// AwaitingView names each node still in review and its observation branch.
// Task grain observes the task's own branch; coarse grain observes its recorded
// aggregate review after verifying task inclusion.
//
// branch_missing says the recorded branch does not resolve locally, which covers
// both a branch deleted after the merge and one that was never created; it points
// at where to look rather than claiming to tell those apart. no_merge_witness
// says the opposite: the branch IS contained in the integration branch, but only
// on that branch's own first-parent line, which a fast-forward merge and a branch
// that never diverged both produce and which no observation separates.
//
// key carries the node's own canonical key, so the noun/key pair an entry emits
// resolves back to the node the entry is about, whichever of the three nouns
// it names.
type AwaitingView struct {
	Noun                string `json:"noun"`
	ID                  string `json:"id"`
	Key                 string `json:"key,omitempty"`
	Branch              string `json:"branch,omitempty"`
	BranchMissing       bool   `json:"branch_missing,omitempty"`
	NoMergeWitness      bool   `json:"no_merge_witness,omitempty"`
	ReviewMissing       bool   `json:"review_missing,omitempty"`
	AggregateIncomplete bool   `json:"aggregate_incomplete,omitempty"`
}

// SyncRefusedData is the BLOCKED data block of a sync refusal: the reason, the
// integration ref sync resolved and where that value came from, and the node
// whose own branch it collided with. An into_unresolved refusal carries the
// reason and source alone, since there is no value and no node to name.
type SyncRefusedData struct {
	Reason string `json:"reason"`
	Into   string `json:"into,omitempty"`
	Source string `json:"source"`
	Noun   string `json:"noun,omitempty"`
	ID     string `json:"id,omitempty"`
	Key    string `json:"key,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// SyncAttestIntoRefusedData is the BLOCKED data block of the other
// --accept-fast-forward refusal, the one that never reaches a node: the ref this
// run resolved as the integration branch, where that value came from, and the
// git.integration_branch the assertion has to be measured against instead
// (absent when the project declares none, which is the same refusal seen from its
// other side). No nodes are listed because none were classified; the command was
// refused before any observation and before any write.
type SyncAttestIntoRefusedData struct {
	Reason   string `json:"reason"`
	Into     string `json:"into,omitempty"`
	Source   string `json:"source"`
	Declared string `json:"declared,omitempty"`
}

// SyncAttestRefusedData is the BLOCKED data block of an --accept-fast-forward
// refusal: the integration branch the assertion was measured against, and every
// named node that is outside the set the flag may cross. The refusal is
// all-or-nothing and runs before any write, so a caller reading this block knows
// nothing was recorded, including for the named nodes that were eligible.
type SyncAttestRefusedData struct {
	Reason string                  `json:"reason"`
	Into   string                  `json:"into,omitempty"`
	Nodes  []SyncAttestRefusedNode `json:"nodes"`
}

// SyncAttestRefusedNode is one named node the assertion cannot be applied to.
// reason is the core's own token ("branch_missing", "not_contained",
// "nothing_to_cross"), carried rather than restated so the envelope and the core
// cannot drift.
//
// recovery is present only on a not_contained node, and only when the core
// found an honest remedy (mutate.Recovery non-nil): the branch's pushed
// remote-tracking tip is itself contained in the integration branch, so
// there is a real fix rather than a guess (issue #43). Its absence is not a
// second reason the human has to interpret; it means exactly what the bare
// refusal already meant before this field existed.
type SyncAttestRefusedNode struct {
	Noun     string        `json:"noun"`
	ID       string        `json:"id"`
	Key      string        `json:"key,omitempty"`
	Branch   string        `json:"branch,omitempty"`
	Reason   string        `json:"reason"`
	Recovery *RecoveryView `json:"recovery,omitempty"`
}

// RecoveryView is the additive recovery object a not_contained refusal carries
// once mutate has confirmed it is honest: the merged remote tip and the
// unmerged local tip (short shas), the upstream ref they were compared
// against, the worktree path holding the branch checked out (omitted when
// nothing does), and steps, the three prescribed commands in order, the same
// three the human-readable block numbers. worktree is the field a machine
// caller reads to tell the two remediation arms apart without parsing steps.
type RecoveryView struct {
	RemoteTip string   `json:"remote_tip"`
	LocalTip  string   `json:"local_tip"`
	Upstream  string   `json:"upstream"`
	Worktree  string   `json:"worktree,omitempty"`
	Steps     []string `json:"steps"`
}

func newSyncCmd() *cobra.Command {
	var into string
	var acceptFF []string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Observe merged task and aggregate review branches and record done crossings.",
		Long: "sync reads local git to observe task branches or submitted aggregate review\n" +
			"branches merged into integration. Only included riding tasks cross on an\n" +
			"aggregate merge; assembling that review branch alone never marks work done.\n" +
			"Features and epics roll up behind completed tasks. A required gate red at\n" +
			"the merge is recorded honestly as a policy_violations entry under advisory\n" +
			"enforcement. sync writes one event per crossing.\n" +
			"\n" +
			"The integration branch is --into, else git.integration_branch from the\n" +
			"manifest, else the branch you are standing on. sync refuses when that\n" +
			"resolves to a task or aggregate review branch, since a branch is always its own\n" +
			"ancestor. A merge commit is observed by ancestry and a squash by content;\n" +
			"a fast-forward merge leaves a repository identical to one where the branch\n" +
			"never diverged, so it is not observed and sync names the node instead.\n" +
			"\n" +
			"--accept-fast-forward <node> is the way out of that, and it is an\n" +
			"assertion rather than a weaker observation: it records the crossing on your\n" +
			"word and writes the basis human_attested, distinct from ancestry, content\n" +
			"and rollup, so an auditor can always tell an asserted crossing from an\n" +
			"observed one. Name the reviewed task, feature, epic, bug, chore, or tweak by id or\n" +
			"key. It crosses only named work whose recorded review\n" +
			"the declared git.integration_branch already contains, on a branch that\n" +
			"still resolves, with something in review to cross; it refuses the whole\n" +
			"command before any write when a named node is outside that set, and refuses\n" +
			"it outright when the branch this run resolved is not the declared\n" +
			"integration branch. It is the human's to type, never the agent's, even on\n" +
			"instruction.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd, into, acceptFF)
		},
	}
	cmd.Flags().StringVar(&into, "into", "",
		"integration branch whose merges are observed (defaults to git.integration_branch, then the current branch)")
	cmd.Flags().StringArrayVar(&acceptFF, "accept-fast-forward", nil,
		"assert the reviewed task, feature, epic, bug, chore, or tweak was merged by fast-forward (repeatable; human only)")
	addContributorFlag(cmd)
	return cmd
}

// runSync loads the tree, runs the sync observation through the core, and renders
// what it found. A resolved integration branch that is a feature's own, one that
// cannot be resolved at all, and a node named on --accept-fast-forward that is
// outside the set the flag may cross are each a BLOCKED business answer (exit 1,
// a data block); a bad owner or an unrunnable git is a tool failure with an error
// block; otherwise the crossings are a SUCCESS.
//
// Every --accept-fast-forward value is resolved to a feature id here, before the
// core runs, so an unknown or mistyped reference is the ordinary misuse envelope
// (exit 2) rather than a business refusal, and the core is handed ids alone.
func runSync(cmd *cobra.Command, into string, acceptFF []string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "sync", err)
	}

	attest, err := resolveAttested(tree, acceptFF)
	if err != nil {
		return emitFailure(cmd, "sync", err)
	}

	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "sync", err)
	}
	rev := vcs.New(tree.Root)
	res, err := mutate.Sync(tree, d, rev, mutate.SyncInput{Into: into, AcceptFastForward: attest})
	if err != nil {
		if data, human, matched := syncRefusedView(tree, err); matched {
			return emitReport(cmd, "sync", false, data, human, 1)
		}
		return emitFailure(cmd, "sync", err)
	}

	view := syncView(res)
	// sync is one of the four human gates (gateMomentFor, readout_gate.go), so
	// its confirmation carries the readout after it. act.Crossed and
	// act.Awaiting carry res.Crossings and res.Awaiting verbatim: sync
	// already computed the basis of each crossing and why a node stayed in
	// review by walking git, and views performs no I/O to recover either a
	// second time, so syncContent (internal/views/readout.go) reads them
	// straight rather than falling back to a bare in_review count.
	act := &views.GateAct{Crossed: syncCrossingsAct(res.Crossings), Awaiting: syncAwaitingAct(res.Awaiting)}
	return emitReportWithReadout(cmd, "sync", true, view, humanSync(tree, res), 0, gateMomentFor("sync"), tree.Root, act)
}

// syncCrossingsAct projects mutate.Sync's own Crossings onto the readout's
// GateActCrossing, the Basis syncContent tallies. mutate.Crossing's own Noun
// and ID are dropped in the translation: nothing downstream ever read them
// back (whole-branch review, finding 3).
func syncCrossingsAct(cs []mutate.Crossing) []views.GateActCrossing {
	out := make([]views.GateActCrossing, len(cs))
	for i, c := range cs {
		out[i] = views.GateActCrossing{Basis: c.Basis, NodeID: c.ID,
			HistoricalPolicyViolations: views.PolicyViolations(store.GatesBlock{PolicyViolations: c.HistoricalPolicyViolations})}
	}
	return out
}

// syncAwaitingAct projects mutate.Sync's own Awaiting onto the readout's
// GateActAwaiting, the same BranchMissing/NoMergeWitness distinction
// humanSyncAwaiting already renders in the confirmation. mutate.Awaiting's
// own Noun, ID, and Branch are dropped in the translation: nothing
// downstream ever read them back (whole-branch review, finding 3).
func syncAwaitingAct(as []mutate.Awaiting) []views.GateActAwaiting {
	out := make([]views.GateActAwaiting, len(as))
	for i, a := range as {
		out[i] = views.GateActAwaiting{
			BranchMissing:  a.BranchMissing,
			NoMergeWitness: a.NoMergeWitness,
			ReviewMissing:  a.ReviewMissing,
		}
	}
	return out
}

// resolveAttested resolves the explicitly reviewed node. Core checks that its
// grain owns the review and that the declared integration branch contains it.
func resolveAttested(tree *store.Tree, refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	r := resolve.NewResolver(tree)
	ids := make([]string, 0, len(refs))
	for _, raw := range refs {
		nouns := append([]string{store.NounFeature, store.NounTask, store.NounEpic}, store.ShelfNouns()...)
		shelf, onShelf := store.ShelfOfRef(raw)
		switch {
		case strings.HasPrefix(raw, "T-"):
			nouns = []string{store.NounTask}
		case strings.HasPrefix(raw, "E-"):
			nouns = []string{store.NounEpic}
		case onShelf:
			nouns = []string{shelf.Noun}
		case id.Valid(raw):
			nouns = []string{store.NounFeature}
		}
		var n store.Node
		var firstErr error
		for _, noun := range nouns {
			resolved, err := r.Resolve(resolve.Ref{Noun: noun, Raw: raw})
			if err == nil {
				n, firstErr = resolved, nil
				break
			}
			if firstErr == nil {
				firstErr = err
			}
		}
		if firstErr != nil {
			return nil, firstErr
		}
		ids = append(ids, n.NodeID())
	}
	return ids, nil
}

// syncRefusedView maps a typed sync refusal onto its data block and human line.
// matched is false for anything that is not a blocked business answer (a bad
// owner, a git that cannot run), which the caller treats as a tool failure. The
// breadcrumbs name only in-project destinations and tiller flags.
func syncRefusedView(tree *store.Tree, err error) (data any, human string, matched bool) {
	var own *mutate.IntoIsFeatureBranchError
	if errors.As(err, &own) {
		d := SyncRefusedData{
			Reason: reasonIntoIsFeatureBranch,
			Into:   own.Into,
			Source: own.Source,
			Noun:   own.Noun,
			ID:     own.NodeID,
			Key:    own.Key,
			Branch: own.Branch,
		}
		human := fmt.Sprintf("Blocked: refusing to observe merges into %s %s's own branch (into is %s, from %s).\n", own.Noun, gen.RefByID(tree, own.NodeID), own.Into, own.Source) +
			"  A branch is always its own ancestor, so this would record a done crossing no merge earned.\n" +
			"  Check out the integration branch, or pass --into <branch>, or set git.integration_branch in .tiller/tiller.yml."
		return d, human, true
	}

	var unres *mutate.IntoUnresolvedError
	if errors.As(err, &unres) {
		d := SyncRefusedData{Reason: reasonIntoUnresolved, Source: intoSourceUnresolved}
		// Two paths reach this reason and only one of them may say the project
		// declares no integration branch. An explicit --into HEAD on a detached
		// checkout is refused for a different fact, and the project it refuses may
		// declare one perfectly well.
		human := "Blocked: cannot tell which branch merges land on; HEAD is detached and no integration branch is set.\n" +
			"  Pass --into <branch>, or set git.integration_branch in .tiller/tiller.yml."
		if unres.Into != "" {
			human = fmt.Sprintf("Blocked: cannot tell which branch merges land on; --into %s names no branch while HEAD is detached.\n", unres.Into) +
				"  Pass --into <branch>, or check out the integration branch."
		}
		return d, human, true
	}

	var missing *mutate.IntegrationMissingError
	if errors.As(err, &missing) {
		d := SyncRefusedData{Reason: reasonIntegrationMissing, Into: missing.Into, Source: missing.Source}
		human := fmt.Sprintf("Blocked: %s (from %s) resolves to no commit, so there is nothing to observe merges into.\n", missing.Into, missing.Source) +
			"  Restore or fetch that branch with ordinary Git, or pass --into <branch>."
		return d, human, true
	}

	var notDeclared *mutate.AttestIntoNotDeclaredError
	if errors.As(err, &notDeclared) {
		d := SyncAttestIntoRefusedData{
			Reason:   reasonIntoNotIntegration,
			Into:     notDeclared.Into,
			Source:   notDeclared.Source,
			Declared: notDeclared.Declared,
		}
		if notDeclared.Declared == "" {
			human := "Blocked: --accept-fast-forward asserts that work landed on the project's integration branch, and this project declares none.\n" +
				fmt.Sprintf("  This run would have measured the assertion against %s (from %s), which is whatever branch happened to be resolved rather than the trunk you merged onto.\n", notDeclared.Into, notDeclared.Source) +
				"  Set git.integration_branch in .tiller/tiller.yml, then re-run. Plain tiller sync is unaffected."
			return d, human, true
		}
		human := fmt.Sprintf("Blocked: --accept-fast-forward asserts about %s, the declared integration branch, and this run resolved %s (from %s).\n",
			notDeclared.Declared, notDeclared.Into, notDeclared.Source) +
			fmt.Sprintf("  Containment in any other ref is a true answer to a different question, so asserting on it would record done for work %s does not carry.\n", notDeclared.Declared) +
			fmt.Sprintf("  Re-run with --into %s, or drop --accept-fast-forward to observe merges into %s.", notDeclared.Declared, notDeclared.Into)
		return d, human, true
	}

	var attest *mutate.AttestNotEligibleError
	if errors.As(err, &attest) {
		d := SyncAttestRefusedData{
			Reason: reasonNotAttestable,
			Into:   attest.Into,
			Nodes:  make([]SyncAttestRefusedNode, 0, len(attest.Nodes)),
		}
		// The headline names the SET, never one of its three conditions. Two of
		// them are about containment and the third, nothing_to_cross, is reachable
		// only once containment has already been established, so a containment
		// headline stated the opposite of the per-node line printed under it.
		lines := []string{fmt.Sprintf(
			"Blocked: %d named node(s) are outside the set --accept-fast-forward may cross against %s.",
			len(attest.Nodes), attest.Into)}
		for _, n := range attest.Nodes {
			view := SyncAttestRefusedNode{
				Noun:   n.Noun,
				ID:     n.NodeID,
				Key:    n.Key,
				Branch: n.Branch,
				Reason: n.Reason,
			}
			line := fmt.Sprintf("  - %s %s", n.Noun, gen.RefByID(tree, n.NodeID))
			if n.Branch != "" {
				line += " on " + n.Branch
			}
			lines = append(lines, line+": "+attestReasonPhrase(n.Reason, attest.Into))

			if n.Recovery != nil {
				steps := recoverySteps(n)
				view.Recovery = &RecoveryView{
					RemoteTip: n.Recovery.RemoteTip,
					LocalTip:  n.Recovery.LocalTip,
					Upstream:  n.Recovery.Upstream,
					Worktree:  n.Recovery.Worktree,
					Steps:     steps,
				}
				lines = append(lines, recoveryHumanLines(tree, attest.Into, n, steps)...)
			}
			d.Nodes = append(d.Nodes, view)
		}
		// The remedy differs by reason, so the trailer restates the candidate set
		// rather than naming one fix: the line above already says which of the
		// three conditions the named node failed.
		lines = append(lines, fmt.Sprintf(
			"  Nothing was recorded, including for the named nodes that were eligible. --accept-fast-forward"+
				" closes only a merge %s already contains, on a branch that still resolves, for a node with"+
				" something in review to cross.", attest.Into))
		return d, strings.Join(lines, "\n"), true
	}

	return nil, "", false
}

// attestReasonPhrase renders one refused node's reason for a terminal reader.
// The tokens are the core's; this is the only place they become English, and an
// unrecognized one degrades to the raw token rather than to silence.
func attestReasonPhrase(reason, into string) string {
	switch reason {
	case mutate.AttestReasonNotContained:
		return "not contained in " + into
	case mutate.AttestReasonBranchMissing:
		return "branch missing locally"
	case mutate.AttestReasonNothingToCross:
		return "nothing in review to cross"
	}
	return reason
}

// recoverySteps renders the three commands a not_contained node's honest
// recovery prescribes, in order: preserve the local tip on a rescue branch so
// the state commit is never lost, realign the working branch onto the pushed
// upstream, then rerun the assertion. It is called once per node and the same
// slice backs both surfaces this file renders: the JSON steps array verbatim,
// and the human block's numbered lines below.
//
// The middle step is the one with two forms. n.Recovery.Worktree is empty
// when nothing holds the branch checked out, and there `git branch -f` is the
// honest command; when a worktree holds it, `git branch -f` refuses outright
// (git will not move a branch checked out anywhere), so the step names
// `git -C <worktree> reset --hard <upstream>` instead, which realigns the
// same ref through the checkout that owns it. That worktree fact is also on
// the JSON object's own worktree field, so a machine caller can tell the two
// forms apart without parsing this text.
func recoverySteps(n mutate.AttestNotEligible) []string {
	r := n.Recovery
	step2 := fmt.Sprintf("realign the branch: git branch -f %s %s", n.Branch, r.Upstream)
	if r.Worktree != "" {
		step2 = fmt.Sprintf("realign the branch: git -C %s reset --hard %s", r.Worktree, r.Upstream)
	}
	return []string{
		fmt.Sprintf("preserve the local tip: git branch recovery/%s-in-review %s", n.Key, r.LocalTip),
		step2,
		fmt.Sprintf("rerun: tiller sync --accept-fast-forward %s", n.Key),
	}
}

// recoveryHumanLines renders one not_contained node's remediation block,
// appended directly beneath its refusal line: what the merged remote tip and
// the local tip actually are, then steps numbered 1 through 3. The worktree
// caveat rides as its own indented line under step 2 rather than folded into
// the step text steps already carries, so the JSON steps array (read by a
// machine that already has the worktree field to explain itself) never
// repeats the same sentence twice.
func recoveryHumanLines(tree *store.Tree, into string, n mutate.AttestNotEligible, steps []string) []string {
	r := n.Recovery
	lines := []string{
		fmt.Sprintf("  recovery for %s on %s:", gen.RefByID(tree, n.NodeID), n.Branch),
		fmt.Sprintf("    the merged remote tip %s is contained in %s; the local tip %s is not.",
			r.RemoteTip, into, r.LocalTip),
	}
	for i, s := range steps {
		lines = append(lines, fmt.Sprintf("    %d. %s", i+1, s))
		if i == 1 && r.Worktree != "" {
			lines = append(lines, "       (the branch is checked out in that worktree; git branch -f would refuse)")
		}
	}
	return lines
}

// syncView projects the core sync result onto the JSON data block.
func syncView(res mutate.SyncResult) SyncView {
	crossings := make([]CrossingView, len(res.Crossings))
	for i, c := range res.Crossings {
		crossings[i] = CrossingView{
			Noun:                       c.Noun,
			ID:                         c.ID,
			Key:                        c.Key,
			Status:                     c.Status,
			Observed:                   c.Observed,
			Basis:                      c.Basis,
			MatchedCommit:              c.MatchedCommit,
			PolicyViolations:           nonNilStrings(c.PolicyViolations),
			HistoricalPolicyViolations: views.PolicyViolations(store.GatesBlock{PolicyViolations: c.HistoricalPolicyViolations}),
			TouchedDesign:              c.TouchedDesign,
		}
		if m := c.EvalContractMismatch; m != nil {
			crossings[i].EvalContractMismatch = &EvalContractMismatchView{Accepted: m.Accepted, Verified: m.Verified}
		}
	}
	var reopened []ReopenedView
	for _, r := range res.Reopened {
		reopened = append(reopened, ReopenedView{Noun: r.Noun, ID: r.ID, Key: r.Key, Status: r.Status})
	}
	var skipped []SkippedView
	for _, sk := range res.Skipped {
		view := SkippedView{Noun: sk.Noun, ID: sk.ID, Key: sk.Key, Path: sk.Path, Surface: sk.Surface}
		if sk.Surface {
			view.Remedy = skippedSurfaceRemedy(sk)
		}
		skipped = append(skipped, view)
	}
	var awaiting []AwaitingView
	for _, a := range res.Awaiting {
		awaiting = append(awaiting, AwaitingView{
			Noun:                a.Noun,
			ID:                  a.ID,
			Key:                 a.Key,
			Branch:              a.Branch,
			BranchMissing:       a.BranchMissing,
			NoMergeWitness:      a.NoMergeWitness,
			ReviewMissing:       a.ReviewMissing,
			AggregateIncomplete: a.AggregateIncomplete,
		})
	}
	var due []CheckDueView
	for _, c := range res.ChecksDue {
		due = append(due, CheckDueView{Noun: c.Noun, ID: c.ID, Key: c.Key})
	}
	return SyncView{
		Crossings:  crossings,
		Reopened:   reopened,
		Skipped:    skipped,
		Into:       res.Into,
		IntoSource: res.IntoSource,
		Awaiting:   awaiting,
		Attested:   res.Attested,
		ChecksDue:  due,
	}
}

// humanSync is the plain-text confirmation for a terminal reader. Each crossing
// names its node by the shared reference convention; the crossing carries only
// id and key, so the reference is composed against the tree loaded before the
// sync (keys and parents do not change during a sync). A
// crossing recorded on content evidence says so inline, because content is the
// weaker of the two and a reader should not have to open the event to learn
// which one it rested on.
//
// A sync that recorded nothing while nodes sit in review names them and the
// likely causes rather than printing the clean-run line, since several real merge
// shapes leave nothing observable behind. The exit code stays 0: a node waiting on
// a merge is a true report of the world, not a refusal by Tiller, and nothing
// about it is the caller's fault to fix.
//
// Crossings the human asserted on --accept-fast-forward render in their own
// block, under their own header, and never inside the observed count. An
// assertion dressed as an observation would be the exact fabrication the
// observation refuses to commit, so the block names every node it crossed with
// no truncation at any limit, prints the basis on each, and says plainly that
// this rests on the human's word.
func humanSync(tree *store.Tree, res mutate.SyncResult) string {
	if len(res.Crossings) == 0 {
		reopened, skipped := humanSyncReopened(tree, res), humanSyncSkipped(tree, res)
		// The clean-run line is suppressed whenever another block has something
		// to say, because "no merged branches to record" directly contradicts a
		// list of nodes this run just moved or could not record. It survives
		// only where it is the whole truth: nothing crossed, nothing moved back,
		// nothing was skipped, and nothing sits in review.
		if len(res.Awaiting) == 0 && (reopened != "" || skipped != "") {
			return joinSyncBlocks(reopened, skipped, humanSyncChecksDue(tree, res))
		}
		return joinSyncBlocks(humanSyncAwaiting(tree, res), reopened, skipped, humanSyncChecksDue(tree, res))
	}
	var observed, asserted []mutate.Crossing
	for _, c := range res.Crossings {
		if c.Basis == basisHumanAttested {
			asserted = append(asserted, c)
			continue
		}
		observed = append(observed, c)
	}

	var lines []string
	if len(observed) > 0 {
		lines = append(lines, fmt.Sprintf("Sync: recorded %d done crossing(s):", len(observed)))
		for _, c := range observed {
			lines = append(lines, syncCrossingLine(tree, c))
		}
	}
	if len(asserted) > 0 {
		lines = append(lines, "Recorded on your assertion, not observation:")
		for _, c := range asserted {
			lines = append(lines, syncCrossingLine(tree, c), "    basis: "+c.Basis)
		}
		// "contains this work" would overstate the observation on the accepted
		// vacuous-branch case, where the branch carries no work at all and what
		// %s contains is a bare tip. Naming the tip is the honest version and is
		// true of the fast-forward case too.
		lines = append(lines, fmt.Sprintf("  Tiller observed only that %s already contains the branch tip, which"+
			" a fast-forward merge and a branch that never diverged both leave, so this rests on your word rather"+
			" than on evidence.", res.Into))
	}
	if touched := humanSyncTouchedDesign(res.Crossings); touched != "" {
		lines = append(lines, touched)
	}
	if mismatch := humanSyncEvalContractMismatch(tree, res.Crossings); mismatch != "" {
		lines = append(lines, mismatch)
	}
	return joinSyncBlocks(strings.Join(lines, "\n"), humanSyncReopened(tree, res), humanSyncSkipped(tree, res),
		humanSyncChecksDue(tree, res))
}

func humanSyncTouchedDesign(crossings []mutate.Crossing) string {
	docs := make(map[string]struct{})
	for _, crossing := range crossings {
		for _, doc := range crossing.TouchedDesign {
			docs["design/"+doc] = struct{}{}
		}
	}
	if len(docs) == 0 {
		return ""
	}
	paths := make([]string, 0, len(docs))
	for doc := range docs {
		paths = append(paths, doc)
	}
	sort.Strings(paths)
	return "as-is docs touched: " + strings.Join(paths, ", ") + "; run tiller drift"
}

// humanSyncEvalContractMismatch names every feature that crossed with an
// integration eval other than the accepted one. The crossing stands; the block
// exists so a completion record naming a contract no gate judged is never
// silent.
func humanSyncEvalContractMismatch(tree *store.Tree, crossings []mutate.Crossing) string {
	var lines []string
	for _, c := range crossings {
		if m := c.EvalContractMismatch; m != nil {
			lines = append(lines, fmt.Sprintf("  - feature %s: accepted %s, integration %s",
				gen.RefByID(tree, c.ID), m.Accepted, m.Verified))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(append([]string{"Eval contract differs from the accepted one (advisory):"}, append(lines,
		"  The completion record pins the integration commit's feature.eval.md; check and submit judged the"+
			" accepted one. Commit the accepted eval to integration, or re-check against what landed.")...), "\n")
}

// humanSyncSkipped names the nodes whose rollup was earned and whose crossing
// could not be recorded, in their own block, because nothing else on any surface
// will ever mention them. Their work is complete and their status is unchanged,
// so without this the run prints its clean line while a node quietly never
// crosses, which is the exact indistinguishability cli-surface.md's done-crossing
// section settles against.
//
// The line names the path rather than a category, since the path is the fix.
//
// It says git could not resolve the path, and stops there, rather than claiming
// the path "is not under version control": the checkout's index is not what was
// read. Every leg of a completion record, the prose blob and each surface
// fingerprint (a git_tree for a directory, a git_blob for a file), is read from
// the one integration commit sync resolved, so the path is missing from that
// commit, whatever the checkout holds. The paragraph below says so, which is
// what turns the line into something a reader can act on.
//
// Since the blob fallback landed, a surface path skips only when it resolves to
// nothing in the integration commit or to an object that is neither tree nor
// blob (a submodule gitlink), so the "once its path does resolve" promise below
// is honest.
func humanSyncSkipped(tree *store.Tree, res mutate.SyncResult) string {
	if len(res.Skipped) == 0 {
		return ""
	}
	lines := []string{fmt.Sprintf("Earned a crossing but could not be recorded (%d):", len(res.Skipped))}
	surfaceSkipped := false
	for _, sk := range res.Skipped {
		lines = append(lines, fmt.Sprintf("  - %s %s: git could not resolve %s",
			sk.Noun, gen.RefByID(tree, sk.ID), sk.Path))
		surfaceSkipped = surfaceSkipped || sk.Surface
	}
	lines = append(lines, "  A completion record pins the node's prose blob and one fingerprint per surface path"+
		" (a git_tree for a directory, a git_blob for a file), all read from the integration commit sync"+
		" observed. A path missing from that commit leaves nothing to record the crossing against."+
		" Each node is unchanged and will cross on the next sync once its path does resolve on integration.")
	if surfaceSkipped {
		lines = append(lines, "  A missing surface path the work itself moved or removed will never resolve again."+
			" Record the code's current location with `tiller link <noun> <node> --surface <path>`, then drop the old path:")
		for _, sk := range res.Skipped {
			if sk.Surface {
				lines = append(lines, "    "+skippedSurfaceRemedy(sk))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// skippedSurfaceRemedy is the unlink that drops a skipped node's missing
// surface path, addressed by key when it has one.
func skippedSurfaceRemedy(sk mutate.Skipped) string {
	addr := sk.ID
	if sk.Key != "" {
		addr = sk.Key
	}
	return fmt.Sprintf("tiller unlink %s %s --surface %s", sk.Noun, addr, shellWord(sk.Path))
}

// shellWord single-quotes a path a shell would split or expand, so a copied
// remedy names the path exactly.
func shellWord(path string) string {
	for _, r := range path {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._/-+@", r)) {
			return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
		}
	}
	return path
}

// humanSyncChecksDue names the features this run leaves awaiting their feature
// check: every task done, a required feature-grain gate with no recorded
// result, the rollup parked until `tiller check feature` runs. It renders on
// every outcome, because the run that crosses a feature's last task is exactly
// the one that used to read as finished while the feature stranded silently
// (docs/plans/2026-07-31-feature-rollup-visibility-design.md, The incident).
// The id is used in the command rather than the key: check addresses features
// by either, and the id is what every other queue command carries.
func humanSyncChecksDue(tree *store.Tree, res mutate.SyncResult) string {
	if len(res.ChecksDue) == 0 {
		return ""
	}
	lines := []string{fmt.Sprintf(
		"Awaiting feature check: %d feature(s) with every task done and required gates unrecorded:",
		len(res.ChecksDue))}
	for _, c := range res.ChecksDue {
		lines = append(lines, fmt.Sprintf("  - feature %s; run: tiller check feature %s",
			gen.RefByID(tree, c.ID), c.ID))
	}
	return strings.Join(lines, "\n")
}

// humanSyncReopened renders the reverse direction, in its own block under its own
// header so it can never be misread as a crossing. It is empty on the ordinary
// sync, which is why the block appears at all only when something moved back:
// this is the one thing a sync does that the developer did not ask for and would
// otherwise discover by reading the file.
func humanSyncReopened(tree *store.Tree, res mutate.SyncResult) string {
	if len(res.Reopened) == 0 {
		return ""
	}
	lines := []string{fmt.Sprintf("Reopened by work that landed beneath them (%d):", len(res.Reopened))}
	for _, r := range res.Reopened {
		lines = append(lines, fmt.Sprintf("  - %s %s -> %s", r.Noun, gen.RefByID(tree, r.ID), r.Status))
	}
	lines = append(lines, "  A feature's and an epic's done is a rollup over their children, so it"+
		" recomputes: new work beneath one returns it to work rather than needing an amendment to reopen it.")
	return strings.Join(lines, "\n")
}

// joinSyncBlocks stitches the confirmation's blocks with a blank line between
// them, skipping the ones that had nothing to say, so a sync with no reopenings
// reads byte for byte as it did before the reverse existed.
func joinSyncBlocks(blocks ...string) string {
	var present []string
	for _, b := range blocks {
		if b != "" {
			present = append(present, b)
		}
	}
	return strings.Join(present, "\n\n")
}

// syncCrossingLine renders one crossed node, named by the shared reference
// convention. Content evidence and policy violations annotate the line the same
// way whichever block it lands in: advisory enforcement does not soften because
// the basis is an assertion, and if anything it matters more there.
func syncCrossingLine(tree *store.Tree, c mutate.Crossing) string {
	line := fmt.Sprintf("  - %s %s -> %s", c.Noun, gen.RefByID(tree, c.ID), c.Status)
	if c.MatchedCommit != "" {
		line += fmt.Sprintf(" (content match: %s)", c.MatchedCommit)
	}
	if len(c.PolicyViolations) > 0 {
		line += fmt.Sprintf(" (policy_violations: %v)", c.PolicyViolations)
	}
	for _, violation := range c.HistoricalPolicyViolations {
		line += fmt.Sprintf("; historical policy %s: %s (source %s)", violation.Gate, violation.Status, violation.SourceRevision.Value)
		if len(violation.UnresolvedCriteria) > 0 {
			line += "; unresolved criteria: " + strings.Join(violation.UnresolvedCriteria, ", ")
		}
		if len(violation.EvidenceRefs) > 0 {
			line += "; evidence: " + strings.Join(violation.EvidenceRefs, ", ")
		}
	}
	return line
}

// humanSyncAwaiting renders the zero-crossing outcome: the clean-run line when
// nothing is waiting, and otherwise the nodes still in review plus what puts them
// there, so a stranded node is never reported as silence.
//
// The causes named are the ones that are actually true. The observation reaches a
// merge commit and a squash; every other way a merge can land leaves either no
// evidence or evidence the branch no longer matches, and a node can equally well
// be sitting there because nobody has merged it yet. Two nodes carry a sharper
// answer than the general list and say so on their own line: a branch that does
// not resolve locally, and a branch contained in the integration branch with no
// merge commit to witness it, which is what a fast-forward leaves.
//
// That second one has a route out and the readout names it, in the third person
// on purpose: the reader may be either actor, and the line hands the command
// over rather than inviting whoever is reading to run it.
func humanSyncAwaiting(tree *store.Tree, res mutate.SyncResult) string {
	if len(res.Awaiting) == 0 {
		return "Sync: no merged branches to record."
	}
	lines := []string{fmt.Sprintf("Sync: no crossings recorded against %s; %d node(s) still in review:",
		res.Into, len(res.Awaiting))}
	stranded := false
	for _, a := range res.Awaiting {
		if !a.NoMergeWitness {
			continue
		}
		stranded = true
	}
	for i, a := range res.Awaiting {
		if i == awaitingHumanLimit {
			lines = append(lines, fmt.Sprintf("  ... and %d more.", len(res.Awaiting)-awaitingHumanLimit))
			break
		}
		line := fmt.Sprintf("  - %s %s", a.Noun, gen.RefByID(tree, a.ID))
		if a.Branch != "" {
			line += " on " + a.Branch
		}
		if a.BranchMissing {
			line += " (branch missing locally)"
		}
		if a.NoMergeWitness {
			line += fmt.Sprintf(" (in %s already, but no merge commit witnesses it)", res.Into)
		}
		if a.ReviewMissing {
			line += " (aggregate review is not recorded; sync has not inspected its merge state)"
		}
		if a.AggregateIncomplete {
			line += " (aggregate does not contain this task's submitted work and current tip)"
		}
		lines = append(lines, line)
		if a.ReviewMissing {
			if task := tree.Tasks[a.ID]; task != nil {
				if owner := workflow.ReviewOwnerOf(tree, task); owner != nil {
					lines = append(lines, "    "+workflow.MissingReviewGuidance(owner))
				}
			}
		}
	}
	lines = append(lines,
		"Not merged yet, or merged in a shape sync cannot read: a fast-forward, a "+
			"rebase-and-merge, a branch deleted or committed to after the merge, or a "+
			"squash the branch no longer matches.")
	if stranded {
		// The assertion targets the node that owns the resolved review.
		lines = append(lines,
			"A node in "+res.Into+" already with no merge commit to witness it crosses only on the "+
				"human's assertion, which is theirs to type: tiller sync --accept-fast-forward <node>.")
		lines = append(lines,
			"  Name the task at task grain, the reviewed feature or epic at coarse grain, or the bug/chore/tweak.")
	}
	return strings.Join(lines, "\n")
}
