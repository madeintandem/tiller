package cli

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/core/review"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file wires the lifecycle verbs (approve, spec, start, return,
// resume, cancel, archive, submit) onto the root command. Each is a thin shell: it validates the
// noun against the verbs the node class allows, resolves the address, hands a
// clean request to internal/core/mutate.Transition, and renders the outcome. The
// reasoning, legality, file writes, and event recording live in the core.
//
// approve is the human start gate: it approves a proposed plan (an epic
// decomposition, a feature spec, a task). Nothing here accepts completed work.
// A task reaches done by the merge sync observes, and a feature's and an epic's
// done are rollups sync computes over their children (cli-surface.md, "The done
// crossing": there is no accept verb). archive, the human's deliberate close of
// a done epic, is the one judgment moment left at the top of the tree.
//
// The outcome is one of three envelopes. A legal transition prints the new node
// state as a SUCCESS (cli-surface.md, "Output and exit codes"). An illegal move,
// or a structural-blocker refusal, is a BLOCKED business answer (exit 1): an
// ok:false report carrying a data block (current_state, legal_next, or the
// blockers), never an error block, so a stuck agent self-corrects without a
// human. A bad address or an environment fault is a tool failure (exit 2/3) with
// an error block. The sync-to-done verb is deliberately absent: it observes a git
// merge, which lands with internal/vcs in Phase 4 (T14).

// TransitionView is the SUCCESS payload of a lifecycle verb: the resulting node
// state the agent reads back. Key is omitted where the node carries none;
// Branch and BaseBranch name the branch recorded at start and its base.
// Tiller reports these Git operations but never performs them; PR appears on a
// task once submit records one; ReviewRef appears instead once a remote-less
// submit records the branch-ref fallback; Changed is false on an idempotent
// no-op so a retry is distinguishable from a fresh move.
type TransitionView struct {
	Noun            string         `json:"noun"`
	ID              string         `json:"id"`
	Key             string         `json:"key,omitempty"`
	Status          string         `json:"status"`
	Branch          string         `json:"branch,omitempty"`
	BaseBranch      string         `json:"base_branch,omitempty"`
	AggregateBranch string         `json:"aggregate_branch,omitempty"`
	AdoptedCommits  []string       `json:"adopted_commits,omitempty"`
	PR              *int           `json:"pr,omitempty"`
	ReviewRef       *ReviewRefView `json:"review_ref,omitempty"`
	Changed         bool           `json:"changed"`

	// Tasks are the tasks the same transition moved beside the node addressed.
	// The feature plan gate is what fills it, since approving a specced feature
	// flips its initial proposed tasks in the same act (cli-surface.md,
	// "Specifying a feature"); every other transition leaves it absent.
	Tasks []TransitionView `json:"tasks,omitempty"`

	// UnpushedReview names the review-state warning (issue #42) when submit
	// just recorded a PR for a task whose own branch already holds a
	// commit that PR cannot show a reviewer: the local tip is not on the
	// remote. Only runTransition's submit arm ever sets it; every other
	// lifecycle verb's TransitionView carries it nil, byte-identical to
	// before this field existed.
	UnpushedReview *views.UnpushedReviewView `json:"unpushed_review,omitempty"`

	// Warnings are advisories the transition succeeded alongside, never
	// refusals. Only `spec` fills it today (a preserve criterion that only
	// repeats the required tests gate); every other verb leaves it absent.
	Warnings []string `json:"warnings,omitempty"`
}

// ReviewedAssumptionView is the SUCCESS payload of approve
// --reviewed-assumption: the feature's identity and the question whose
// answer was reviewed in. It carries no Status, unlike TransitionView,
// because reviewing an assumption in is deliberately not a lifecycle
// transition (mutate.ReviewAssumption's own doc comment); Changed is false
// on the idempotent no-op where the stamp already matched.
type ReviewedAssumptionView struct {
	Noun       string `json:"noun"`
	ID         string `json:"id"`
	Key        string `json:"key,omitempty"`
	QuestionID string `json:"question_id"`
	Changed    bool   `json:"changed"`
}

// ReviewRefView is the branch-ref review surface a remote-less submit records
// where a PR would otherwise be: the branch review happens on and the head
// commit value ("git:<short-sha>") at submission.
type ReviewRefView struct {
	Branch string `json:"branch"`
	Head   string `json:"head"`
}

// reviewRefView projects the stored review ref onto the JSON view; a node with
// none (the normal, PR-backed case) stays absent from the envelope.
func reviewRefView(r *store.ReviewRef) *ReviewRefView {
	if r == nil {
		return nil
	}
	return &ReviewRefView{Branch: r.Branch, Head: r.Head.Value}
}

// IllegalTransitionData is the BLOCKED data block for an illegal move: the verb
// that was refused, the current state, and the moves legal from here, which is
// exactly what the agent needs to correct itself.
type IllegalTransitionData struct {
	Noun         string   `json:"noun"`
	ID           string   `json:"id"`
	Key          string   `json:"key,omitempty"`
	Reason       string   `json:"reason"`
	Verb         string   `json:"verb"`
	CurrentState string   `json:"current_state"`
	LegalNext    []string `json:"legal_next"`
}

// QuestionBlockerView is one open question standing in the way of a start or
// submit. Title is the short scan handle; Statement remains the full structured
// detail. TargetID and TargetNoun name the nearest canonical matched node.
type QuestionBlockerView struct {
	Question   string   `json:"question"`
	Title      string   `json:"title,omitempty"`
	Statement  string   `json:"statement,omitempty"`
	Blocks     []string `json:"blocks,omitempty"`
	TargetID   string   `json:"target_id,omitempty"`
	TargetNoun string   `json:"target_noun,omitempty"`
}

// BlockedByQuestionsData is the BLOCKED data block for a structural-blocker
// refusal: the open questions the node must answer or assume before it can move.
type BlockedByQuestionsData struct {
	Noun         string                `json:"noun"`
	ID           string                `json:"id"`
	Reason       string                `json:"reason"`
	CurrentState string                `json:"current_state"`
	Blockers     []QuestionBlockerView `json:"blockers"`
}

// SpecIncompleteData is the BLOCKED data block for a spec refusal: the spec or
// eval files that are not yet written.
type SpecIncompleteData struct {
	Noun         string   `json:"noun"`
	ID           string   `json:"id"`
	Reason       string   `json:"reason"`
	CurrentState string   `json:"current_state"`
	Incomplete   []string `json:"incomplete"`
}

// ParentPlanUngatedData is the BLOCKED data block for a doing task verb
// refused because the parent feature's plan has not crossed the plan gate
// (mutate.ParentPlanUngatedError; issue #33). MissingSeam carries the command
// the human (or agent) runs next, the machine half of the human line.
// FeatureKey rides beside Feature (the id) additively (issue #35): both
// parent-guard blocks now carry the feature's canonical key as its own
// structured field, the id-and-key rule the rest of the envelopes already keep.
type ParentPlanUngatedData struct {
	Noun         string `json:"noun"`
	ID           string `json:"id"`
	Reason       string `json:"reason"`
	CurrentState string `json:"current_state"`
	Feature      string `json:"feature"`
	FeatureKey   string `json:"feature_key"`
	FeatureState string `json:"feature_state"`
	MissingSeam  string `json:"missing_seam"`
}

// ParentCancelledData is the BLOCKED data block for a doing task verb refused
// because the parent feature is cancelled (mutate.ParentCancelledError; issue
// #35). MissingSeam names the remedy that is always reachable: cancel is not
// a doing verb, so retiring the task works under any parent state.
type ParentCancelledData struct {
	Noun         string `json:"noun"`
	ID           string `json:"id"`
	Reason       string `json:"reason"`
	CurrentState string `json:"current_state"`
	Feature      string `json:"feature"`
	FeatureKey   string `json:"feature_key"`
	MissingSeam  string `json:"missing_seam"`
}

// ParentEpicRetiredData is the BLOCKED data block for a doing verb refused
// because an ancestor epic is retired (mutate.ParentEpicRetiredError; issue
// #36). The remedy differs from the feature arm's only in grain: retire the
// node too, or re-add the epic and plan the work anew; MissingSeam is a
// template for the same reason the cancelled arm's is (the reason is the
// human's to write).
type ParentEpicRetiredData struct {
	Noun         string `json:"noun"`
	ID           string `json:"id"`
	Reason       string `json:"reason"`
	CurrentState string `json:"current_state"`
	Epic         string `json:"epic"`
	EpicKey      string `json:"epic_key"`
	EpicState    string `json:"epic_state"`
	MissingSeam  string `json:"missing_seam"`
}

// ActiveBasisData is the BLOCKED data block for an active-basis refusal. Drifted
// names the files whose current WORKING-TREE content no longer matches what was
// pinned, which is what lets an edit be caught before it is committed; Missing
// names the pinned files that are absent from the working tree
// (mutate.ActiveBasisError documents each field and where their relationship is
// decided). Noun and ID name the node whose basis moved, a feature (feature.md,
// feature.eval.md) or a task (task.md). The node named is not always the node the
// caller acted on: a task's start, check, or submit compares its parent feature's
// pins too (mutate.guardTask).
type ActiveBasisData struct {
	Noun         string   `json:"noun"`
	ID           string   `json:"id"`
	Reason       string   `json:"reason"`
	CurrentState string   `json:"current_state"`
	Drifted      []string `json:"drifted"`
	Missing      []string `json:"missing,omitempty"`
}

// AnsweredQuestionView is one unreviewed assumption a submit refusal names.
// Title lets scan-oriented clients label it compactly; Statement stays because
// the answer/assumption review contract needs the full evidence under review.
type AnsweredQuestionView struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Statement string `json:"statement,omitempty"`
}

// AnsweredQuestionData is the BLOCKED data block for submit's assumption-review
// refusal (cli-surface.md, the guard paragraph: "submit alone adds the
// assumption review"). Noun and ID name the node whose assumption_basis
// carries the entry, always the feature (mutate.AnsweredQuestionError
// documents the walk-up), never the task submit was run on. Remedy is the
// exact runnable command(s) that clear every question named, addressing that
// carrying feature: a reader handed a task command for a feature's entry gets
// a refusal loop (task-6-brief.md, resolution 3).
type AnsweredQuestionData struct {
	Noun      string                 `json:"noun"`
	ID        string                 `json:"id"`
	Reason    string                 `json:"reason"`
	Questions []AnsweredQuestionView `json:"questions"`
	Remedy    string                 `json:"remedy"`
}

// ClaimHeldData identifies an execution unit's existing reservation. Seam
// names the explicit --from command for a human-authorized pre-start handoff.
type ClaimHeldData struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Reason string `json:"reason"`
	Holder string `json:"holder"`
	Seam   string `json:"seam"`
}

// ClaimHolderMismatchData is the BLOCKED data block for a --from that names
// no legitimate takeover target (mutate.ClaimHolderMismatchError). Holder
// and Seam are both empty when the unit is unclaimed outright (there is
// nothing to compose a retry from); otherwise Holder names the real holder
// and Seam is the corrected --from.
type ClaimHolderMismatchData struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Reason string `json:"reason"`
	Holder string `json:"holder,omitempty"`
	Seam   string `json:"seam,omitempty"`
}

// NotClaimantData is the BLOCKED data block for `release feature`'s own
// refusal (mutate.NotClaimantError): releasing a feature the acting contributor
// does not hold. Seam is the full reassign command, the design's own remedy
// for handing work off without the claimant's cooperation.
type NotClaimantData struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Reason string `json:"reason"`
	Holder string `json:"holder"`
	Seam   string `json:"seam"`
}

// WorkUnclaimedData names an unclaimed execution unit.
type WorkUnclaimedData struct {
	Noun         string `json:"noun"`
	ID           string `json:"id"`
	Key          string `json:"key,omitempty"`
	Reason       string `json:"reason"`
	CurrentState string `json:"current_state"`
	Seam         string `json:"seam"`
}

// WorkClaimHeldData names an execution unit held by another developer.
type WorkClaimHeldData struct {
	Noun         string `json:"noun"`
	ID           string `json:"id"`
	Key          string `json:"key,omitempty"`
	Reason       string `json:"reason"`
	CurrentState string `json:"current_state"`
	Holder       string `json:"holder"`
	Seam         string `json:"seam"`
	ReassignSeam string `json:"reassign_seam"`
}

// newApproveCmd builds `approve <noun> <node>...`, the start gate: the human
// approving a proposed plan. Tiller computes the target from the current state,
// so the same verb approves an epic decomposition (draft->ready), a feature spec
// (specced->ready), and a task (proposed->ready); the state disambiguates the
// gate, so no flag is needed. approve feature --refresh-basis re-records the
// accepted spec/eval revisions without a state move. approve <noun> <node>
// --reviewed-assumption <Q-id> stamps an answered question's revision into the
// node's assumption_basis entry, reviewing it in without changing state either
// (artifact-schema.md, Assumption basis).
//
// approve takes one or more nodes of the noun. A single node keeps the
// single-node envelope exactly; two or more route through the batch arm
// (batch.go): all addresses resolve before anything mutates, then the
// transition applies per node in argument order. approve task --feature <key>
// approves every proposed task on that feature and is mutually exclusive with
// positional nodes. --reviewed-assumption is its own dedicated act (not a
// transition at all: see runApproveReviewedAssumption), so it is mutually
// exclusive with --refresh-basis, with --feature, and with more than one
// positional node: one node and one act at a time.
func newApproveCmd() *cobra.Command {
	var refreshBasis bool
	var feature string
	var reviewedAssumption string
	var reviewGrain string
	var assumeQuestion, assumptionReason string
	allowed := []string{store.NounEpic, store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak}
	cmd := &cobra.Command{
		Use:   "approve <noun> <node>...",
		Short: "Approve one or more proposed plans across their start gate.",
		Long: "approve is the human start gate: it approves a proposed plan. Tiller\n" +
			"computes the target from the current state: an epic decomposition\n" +
			"draft->ready, a feature plan specced->ready, a task proposed->ready.\n" +
			"Approving a specced feature approves the whole plan: its initial\n" +
			"proposed tasks move to ready in the same act, and the accepted basis is\n" +
			"recorded on the feature and on each task.\n" +
			"Several nodes approve in one invocation: every address must resolve\n" +
			"before anything mutates, then each node transitions in argument order\n" +
			"(a blocked node does not stop the rest). approve task\n" +
			"--feature <key> approves every proposed task on that feature. approve\n" +
			"--refresh-basis re-records the accepted revisions of a feature or a task\n" +
			"without changing state. approve <noun> <node> --reviewed-assumption\n" +
			"<Q-id> stamps an answered question's revision into the node's\n" +
			"assumption_basis entry, one node at a time. approve amendment <addr> is\n" +
			"a different act under the same verb: it validates every staged\n" +
			"operation's preconditions against the live tree and, if all hold,\n" +
			"applies the whole batch atomically; it takes exactly one address and\n" +
			"none of the flags above.\n" +
			"approve design <doc>... confirms named design docs against the current code and project HEAD.\n" +
			"All named docs are confirmed atomically; design approval never edits prose or changes work status,\n" +
			"and accepts none of the work-node flags or --all.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("assume-question") || cmd.Flags().Changed("reason") {
				if len(args) != 2 || args[0] != store.NounFeature || strings.TrimSpace(assumeQuestion) == "" || strings.TrimSpace(assumptionReason) == "" ||
					cmd.Flags().Changed("refresh-basis") || cmd.Flags().Changed("review-grain") || cmd.Flags().Changed("reviewed-assumption") || cmd.Flags().Changed("feature") {
					return emitFailure(cmd, "approve", &usageError{reason: "bad_args", msg: "record an assumption separately with approve feature <node> --assume-question <Q-id> --reason <text>"})
				}
				tree, node, err := resolveForMutation(cmd, "approve", args[0], args[1], []string{store.NounFeature})
				if err != nil {
					return err
				}
				d, err := deps(cmd, tree.Root)
				if err != nil {
					return emitFailure(cmd, "approve", err)
				}
				res, err := mutate.Transition(tree, d, node, mutate.TransitionInput{Verb: "approve", AssumeQuestion: assumeQuestion, Reason: assumptionReason})
				if err != nil {
					return reportTransitionError(cmd, "approve", tree, node, err)
				}
				view := map[string]any{"noun": res.Noun, "id": res.ID, "key": res.Key, "status": res.Status, "changed": res.Changed, "assumed_question": assumeQuestion}
				return emitSuccess(cmd, "approve", view, fmt.Sprintf("Feature %s: recorded assumption on %s; state remains %s.", gen.Ref(tree, node), assumeQuestion, res.Status))
			}
			if len(args) >= 1 && args[0] == store.NounDesign {
				return runApproveDesign(cmd, args[1:])
			}
			// An amendment's approve is not a lifecycle transition (it
			// applies a whole staged batch atomically, mutate.ApproveAmendment)
			// and never reaches the noun set runTransition/runTransitionBatch
			// drive below, so it is intercepted first (amendment_cmds.go's
			// runApproveAmendmentDispatch).
			if len(args) >= 1 && args[0] == store.NounAmendment {
				return runApproveAmendmentDispatch(cmd, args, refreshBasis, feature, reviewedAssumption,
					cmd.Flags().Changed("reviewed-assumption"))
			}
			// --review-grain's own validation runs as one block ahead of
			// every other arm: gate on whether the flag was passed at all,
			// not on a non-empty value (the same rule --reviewed-assumption
			// draws immediately below, for the identical reason), then
			// refuse every combination the mutate layer would otherwise
			// silently drop rather than apply. --refresh-basis is
			// controller ruling B: neither transitionEpic, transitionFeature's
			// own refresh-basis arm, nor transitionTask's ever reads
			// ReviewGrain on that path, so combining the two would record
			// the basis refresh and quietly leave the grain unrecorded, the
			// same silent-pass-over class tiller refuses elsewhere.
			// --reviewed-assumption is the identical hazard one flag over:
			// runApproveReviewedAssumption is a dedicated act that never
			// reads ReviewGrain either. Task noun is out of contract on its
			// own terms: store.Task carries no review_grain field at all, so
			// transitionTask never reads the flag either, and letting it
			// through would be the same silent drop under a third name.
			if cmd.Flags().Changed("review-grain") {
				if strings.TrimSpace(reviewGrain) == "" {
					return emitFailure(cmd, "approve", &usageError{
						reason: "bad_args",
						msg:    "--review-grain needs a value (task, feature, or epic); an empty value names none",
					})
				}
				if refreshBasis {
					return emitFailure(cmd, "approve", &usageError{
						reason: "bad_args",
						msg: "--refresh-basis and --review-grain are mutually exclusive: the refresh-basis " +
							"path never records a grain change, so run them as separate acts",
					})
				}
				if cmd.Flags().Changed("reviewed-assumption") {
					return emitFailure(cmd, "approve", &usageError{
						reason: "bad_args",
						msg:    "--review-grain is mutually exclusive with --reviewed-assumption; one node and one act at a time",
					})
				}
				if len(args) > 0 {
					if sing, ok := singularNoun(args[0]); ok && sing == store.NounTask {
						return emitFailure(cmd, "approve", &usageError{
							reason: "bad_args",
							msg:    "--review-grain applies to approve epic or approve feature; a task carries no review grain of its own",
						})
					}
				}
			}
			// Gate on whether the flag was passed at all, not on a
			// non-empty value: `--reviewed-assumption ""` read by value
			// alone falls through into the ordinary transition arm below
			// and crosses the approve gate silently (verified: a specced
			// feature moved to ready on that input). An explicit empty
			// value is misuse, not "flag absent".
			if cmd.Flags().Changed("reviewed-assumption") {
				if strings.TrimSpace(reviewedAssumption) == "" {
					return emitFailure(cmd, "approve", &usageError{
						reason: "bad_args",
						msg:    "--reviewed-assumption needs a question id (Q-xxxxxx); an empty value names none",
					})
				}
				return runApproveReviewedAssumption(cmd, args, feature, refreshBasis, reviewedAssumption)
			}
			in := mutate.TransitionInput{Verb: "approve", RefreshBasis: refreshBasis}
			if cmd.Flags().Changed("review-grain") {
				g := reviewGrain
				in.ReviewGrain = &g
			}
			if feature != "" {
				if len(args) > 1 {
					return emitFailure(cmd, "approve", &usageError{
						reason: "bad_args",
						msg:    "--feature is mutually exclusive with positional nodes; pass one or the other",
					})
				}
				if sing, ok := singularNoun(args[0]); !ok || sing != store.NounTask {
					return emitFailure(cmd, "approve", &usageError{
						reason: "bad_args",
						msg:    fmt.Sprintf("--feature applies to `approve task`, not %q", args[0]),
					})
				}
				return runApproveTasksByFeature(cmd, feature, in)
			}
			if len(args) < 2 {
				return emitFailure(cmd, "approve", &usageError{
					reason: "bad_args",
					msg:    "approve needs at least one node (or `approve task --feature <key>`)",
				})
			}
			if len(args) == 2 {
				return runTransition(cmd, "approve", args[0], args[1], allowed, in)
			}
			return runTransitionBatch(cmd, "approve", args[0], args[1:], allowed, in)
		},
	}
	cmd.Flags().StringVar(&assumeQuestion, "assume-question", "", "record an open question as a feature-only assumption without changing state")
	cmd.Flags().StringVar(&assumptionReason, "reason", "", "human-directed reason for recording the assumption")
	cmd.Flags().BoolVar(&refreshBasis, "refresh-basis", false, "re-record the accepted revisions without changing state (feature or task)")
	cmd.Flags().StringVar(&feature, "feature", "", "approve every proposed task on this feature (task noun only)")
	cmd.Flags().StringVar(&reviewedAssumption, "reviewed-assumption", "",
		"stamp an answered question's revision into the node's assumption_basis entry (feature only; one node at a time)")
	cmd.Flags().StringVar(&reviewGrain, "review-grain", "",
		"record the review grain in the same act as the gate (epic or feature only; task, feature, or epic)")
	addContributorFlag(cmd)
	return cmd
}

// runApproveReviewedAssumption routes `approve <noun> <node>
// --reviewed-assumption <Q-id>`, the dedicated clearing act for
// answered_question drift (artifact-schema.md, Assumption basis;
// mutate.ReviewAssumption's own doc comment). It is deliberately not folded
// into the general transition arm above: reviewing an assumption in changes
// no lifecycle state, so it needs neither --refresh-basis's own bookkeeping
// nor the batch machinery, and mixing either in is refused before anything
// resolves, one node and one act at a time. The noun stays validated against
// the same allowed set every other approve arm uses; a wrong grain (an epic
// or a task, which carries no assumption_basis field) is refused by
// mutate.ReviewAssumption itself with reason unknown_assumption, the same
// division of labor SetSurface draws for its own wrong-grain case.
func runApproveReviewedAssumption(cmd *cobra.Command, args []string, feature string, refreshBasis bool, questionID string) error {
	if refreshBasis {
		return emitFailure(cmd, "approve", &usageError{
			reason: "bad_args",
			msg:    "--reviewed-assumption is mutually exclusive with --refresh-basis; one node and one act at a time",
		})
	}
	if feature != "" {
		return emitFailure(cmd, "approve", &usageError{
			reason: "bad_args",
			msg:    "--reviewed-assumption is mutually exclusive with --feature; one node and one act at a time",
		})
	}
	if len(args) != 2 {
		return emitFailure(cmd, "approve", &usageError{
			reason: "bad_args",
			msg:    "--reviewed-assumption takes exactly one node; one node and one act at a time",
		})
	}
	tree, n, err := resolveForMutation(cmd, "approve", args[0], args[1],
		[]string{store.NounEpic, store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak})
	if err != nil {
		return err
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "approve", err)
	}
	res, err := mutate.ReviewAssumption(tree, d, n, questionID)
	if err != nil {
		return emitFailure(cmd, "approve", err)
	}
	view := ReviewedAssumptionView{
		Noun: res.Noun, ID: res.ID, Key: res.Key, QuestionID: res.QuestionID, Changed: res.Changed,
	}
	ref := gen.Ref(tree, n)
	human := fmt.Sprintf("%s %s: assumption on %s reviewed in; its answer revision is now recorded.",
		res.Noun, ref, questionID)
	if !res.Changed {
		human = fmt.Sprintf("%s %s: assumption on %s is already reviewed in at the current answer revision (no change).",
			res.Noun, ref, questionID)
	}
	// approve is one of the four human gates that print the readout block
	// after their confirmation (cli-surface.md: "Four human gates print the
	// block themselves..."); --refresh-basis already routes through this
	// same emitTransitionSuccess path with its own GateAct arm
	// (transitionAct/approveContent's BasisRefresh branch), and this act
	// needs the identical treatment for the identical reason: it is not a
	// lifecycle transition, so the plain Nodes-tally rendering would claim a
	// move ("Moved to ready") that did not happen.
	act := &views.GateAct{AssumptionReviewed: &views.GateActAssumptionReviewed{
		NodeID: res.ID, QuestionID: res.QuestionID, Changed: res.Changed,
	}}
	return emitTransitionSuccess(cmd, "approve", tree.Root, gateMomentFor("approve"), act, view, human)
}

func newSpecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec feature <node>",
		Short: "Assert a feature's whole specify-stage output is written (draft->specced).",
		Long: "spec feature is the agent asserting the second decompose stage is written:\n" +
			"the specification in feature.md, the acceptance contract in\n" +
			"feature.eval.md, and the task explosion under tasks/. Tiller validates all\n" +
			"three (a missing or stub file is blocked, a malformed contract is misuse),\n" +
			"then moves the feature draft->specced.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransition(cmd, "spec", args[0], args[1],
				[]string{store.NounFeature},
				mutate.TransitionInput{Verb: "spec"})
		},
	}
	addContributorFlag(cmd)
	return cmd
}

func newStartCmd() *cobra.Command {
	var adoptAggregate bool
	cmd := &cobra.Command{
		Use:   "start <task|bug|chore|tweak> <node>",
		Short: "Start claimed execution work and permanently fix its owner.",
		Long:  "Start a claimed task, bug, chore, or tweak. The first task start atomically activates its approved parent feature and records task and aggregate branch identities. Parent guards still apply. Feature-only assumptions are recorded separately with approve feature --assume-question <Q-id> --reason <text>; task start never inherits them. --adopt-aggregate records an existing local ref at the derived aggregate name instead of minting one; inspect git log <integration>..<ref> first.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransition(cmd, "start", args[0], args[1],
				[]string{store.NounTask, store.NounBug, store.NounChore, store.NounTweak}, mutate.TransitionInput{Verb: "start", AdoptAggregate: adoptAggregate})
		},
	}
	cmd.Flags().BoolVar(&adoptAggregate, "adopt-aggregate", false, "record the existing ref at this feature's or epic's derived aggregate name as its aggregate; review its history first")
	addContributorFlag(cmd)
	return cmd
}

func newReturnCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "return <noun> <node>",
		Short: "Reverse a mistaken pre-work gate (feature ready->specced, task ready->proposed).",
		Long: "return is the narrow pre-work reverse: a feature ready->specced, a task\n" +
			"ready->proposed, with a required reason. It is refused once work has\n" +
			"started; those cases route through resume, cancel, or an amendment.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransition(cmd, "return", args[0], args[1],
				[]string{store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak},
				mutate.TransitionInput{Verb: "return", Reason: reason})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why the gate is being reversed (required)")
	_ = cmd.MarkFlagRequired("reason")
	addContributorFlag(cmd)
	return cmd
}

// newResumeCmd builds `resume <task|bug|chore|tweak> <node>`, the path back from
// review. It is task-grade only because a PR always represents a task, a
// bug, or a chore: the review that asked for changes is that node's own PR,
// and a task's feature simply stays active (artifact-schema.md, "A PR always
// represents a task"; docs/plans/2026-08-28-bugs-and-chores-design.md, a bug
// or chore records its own PR).
func newResumeCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "resume <task|bug|chore|tweak> <node>",
		Short: "Re-enter work from review (in_review -> in_progress).",
		Long: "resume is the path back from review when the work itself needs more: a\n" +
			"task, bug, chore, or tweak moves in_review->in_progress. The recorded PR stays.\n" +
			"A command gate whose recorded code identity still equals the current one\n" +
			"keeps its result, so the next check reuses it; every attestation gate and\n" +
			"every gate over other code flips to stale (every gate, when the code\n" +
			"identity cannot be read). A reason is required. A bug or chore resumes\n" +
			"the same way; its PR is its own. A task's feature simply\n" +
			"stays active: a requested-changes review routes the task back to work\n" +
			"without touching the feature.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransition(cmd, "resume", args[0], args[1],
				[]string{store.NounTask, store.NounBug, store.NounChore, store.NounTweak},
				mutate.TransitionInput{Verb: "resume", Reason: reason})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why work is re-entering from review (required)")
	_ = cmd.MarkFlagRequired("reason")
	addContributorFlag(cmd)
	return cmd
}

// newCancelCmd builds `cancel <noun> <node>`, the human terminating a node: an
// epic, feature, or task moves to cancelled from the states the lifecycle
// table allows (including an in_progress task started by mistake). --reason is
// required and recorded on the event, the same way return and resume require
// theirs: cancel is terminal and is the only removal verb, so a cancelled node
// whose reason went unrecorded is a dead branch no later reader can explain.
func newCancelCmd() *cobra.Command {
	var reason, resolution, duplicateOf string
	cmd := &cobra.Command{
		Use:   "cancel <noun> <node>",
		Short: "Terminate a node or an open question (-> cancelled).",
		Long: "cancel is the human terminating a node: an epic, feature, or task\n" +
			"moves to cancelled from the states the lifecycle allows. A --reason is\n" +
			"required and recorded on the event. A bug, chore, or tweak also needs\n" +
			"--resolution wontfix|invalid|duplicate, and duplicate names the\n" +
			"survivor with --duplicate-of. cancel question closes an open question\n" +
			"that no longer needs a decision, releasing every hold it had; an\n" +
			"answered question refuses, and a cancelled one is a no-op success.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sing, _ := singularNoun(args[0])
			if !store.IsWorkNoun(sing) && (resolution != "" || duplicateOf != "") {
				return emitFailure(cmd, "cancel", &usageError{reason: "bad_args",
					msg: "--resolution and --duplicate-of apply to cancel bug|chore|tweak only"})
			}
			if sing == store.NounQuestion {
				return runCancelQuestion(cmd, args[1], reason)
			}
			return runTransition(cmd, "cancel", args[0], args[1],
				[]string{store.NounEpic, store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak},
				mutate.TransitionInput{Verb: "cancel", Reason: reason, Resolution: resolution, DuplicateOf: duplicateOf})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why the node is being cancelled (required)")
	_ = cmd.MarkFlagRequired("reason")
	cmd.Flags().StringVar(&resolution, "resolution", "", "bug|chore|tweak only: wontfix, invalid, or duplicate (required there)")
	cmd.Flags().StringVar(&duplicateOf, "duplicate-of", "", "bug|chore|tweak only, with --resolution duplicate: the surviving node (id or key)")
	addContributorFlag(cmd)
	return cmd
}

// newArchiveCmd builds `archive <epic>`, the shelving move after the rollup: a
// done epic moves to archived. archive targets exactly one node type, so it
// takes the bare epic id or key per the grammar (cli-surface.md, "The grammar").
func newArchiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive <epic>",
		Short: "Move a done epic to archived.",
		Long: "archive shelves a completed epic: done -> archived. It only ever operates\n" +
			"on an epic, so it takes the bare epic id or key.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransition(cmd, "archive", store.NounEpic, args[0],
				[]string{store.NounEpic},
				mutate.TransitionInput{Verb: "archive"})
		},
	}
	addContributorFlag(cmd)
	return cmd
}

// submitNouns are submit's legal target nouns, in the order the design docs
// introduce them one grain at a time: a task's own PR, then the two
// coarse-grain siblings review grain adds (docs/plans/2026-08-26-review-grain-
// design.md, "What a coarse grain changes"), then the two shelf nouns, which
// always record their own PR or review ref with no grain check
// (docs/plans/2026-08-28-bugs-and-chores-design.md).
var submitNouns = []string{store.NounTask, store.NounFeature, store.NounEpic, store.NounBug, store.NounChore, store.NounTweak}

// newSubmitCmd builds `submit <task|feature|epic|bug|chore|tweak> <node> [--pr
// <n>]`. At task grain (the default) a PR always represents a task, so
// `submit task` is the normal path: it records the pull request number and
// moves the task check->in_review, the PR being the review surface. `submit
// feature` and `submit epic` are the coarse-grain siblings review grain adds
// (mutate.SubmitNode): record-only, they never move the reviewed node's own
// status (that is still sync's rollup to observe), and they refuse unless
// the resolved review grain actually names that noun. `submit bug` and
// `submit chore` move the same way `submit task` does (check->in_review),
// but carry no review grain at all: a shelf node always records its own PR
// or review ref (docs/plans/2026-08-28-bugs-and-chores-design.md).
//
// Every noun shares the same remote-less fallback: a repo with no git remote
// (a dry run, local exploration) has nowhere to open a PR, so submit without
// --pr records the current branch and head commit as the review ref instead,
// through offlineReviewRef. With a remote configured, omitting --pr is
// ordinarily a misuse (exit 2), never a silent fallback; the one exception is
// a task whose resolved review grain is feature or epic, where the task
// itself carries no PR at all, remote or not (docs/plans/2026-08-26-review-
// grain-design.md, line 37): there the remote-less fallback is still legal,
// and an explicit --pr is refused instead, since the design's own "never a
// PR" leaves no reading under which a coarse-grain task would carry one.
func newSubmitCmd() *cobra.Command {
	var pr int
	var reason, assume, recoveryHead, recoveryResult string
	var recoverMerged bool
	cmd := &cobra.Command{
		Use:   "submit <task|feature|epic|bug|chore|tweak> <node> [--pr <n>]",
		Short: "Record the review surface and move work to review.",
		Long: "submit records the review surface: the pull request number, or, with\n" +
			"no git remote, the current branch and head commit as a review ref.\n" +
			"submit task moves the task check->in_review; it runs the structural-\n" +
			"blocker guard first, the same as start. submit feature and submit epic\n" +
			"are the coarse-grain siblings review grain adds: record-only, they\n" +
			"leave the reviewed node's own status untouched (sync's rollup still\n" +
			"observes the merge), and they refuse unless the resolved review grain\n" +
			"actually names that noun. A task whose resolved grain is feature or\n" +
			"epic carries no PR of its own at all: its own submit still moves it to\n" +
			"in_review, but only ever records a review ref, remote or not. A bug or\n" +
			"chore or tweak always records its own PR or review ref; review grain does not\n" +
			"apply to it.\n\n" +
			"For explicitly authorized historical feature/epic recovery, run from the\n" +
			"configured integration checkout with --recover-merged --head <reviewed-oid>\n" +
			"--merge-commit <result-oid> --reason <explanation>. Recovery records only\n" +
			"binding and audit evidence; it does not move statuses or approve gates.\n" +
			"A configured remote requires positive --pr, supplied and host-verified by\n" +
			"the caller; Tiller verifies local Git evidence, not the host association.\n" +
			"Commit the actual recovery delta on integration, then run ordinary sync.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sing, ok := singularNoun(args[0])
			if !ok || !containsNoun(submitNouns, sing) {
				return emitFailure(cmd, "submit", &usageError{
					reason: "unknown_noun",
					msg: fmt.Sprintf("submit does not apply to %q (want one of %s)",
						args[0], strings.Join(submitNouns, ", ")),
				})
			}
			recoveryFlags := cmd.Flags().Changed("recover-merged") || cmd.Flags().Changed("head") || cmd.Flags().Changed("merge-commit")
			if recoveryFlags {
				if !recoverMerged || (sing != store.NounFeature && sing != store.NounEpic) {
					return emitFailure(cmd, "submit", &usageError{reason: "recovery_flags_misuse", msg: "--recover-merged, --head, and --merge-commit require enabled recovery on submit feature or submit epic"})
				}
				if cmd.Flags().Changed("assume-question") {
					return emitFailure(cmd, "submit", &usageError{reason: "recovery_flags_misuse", msg: "--assume-question is task submission only, not historical recovery"})
				}
				if !cmd.Flags().Changed("head") || !cmd.Flags().Changed("merge-commit") || strings.TrimSpace(recoveryHead) == "" || strings.TrimSpace(recoveryResult) == "" || strings.TrimSpace(reason) == "" {
					return emitFailure(cmd, "submit", &usageError{reason: "recovery_args_required", msg: "recovery requires --head, --merge-commit, and a nonblank --reason"})
				}
				return runRecoverReview(cmd, sing, args[1], pr, cmd.Flags().Changed("pr"), recoveryHead, recoveryResult, reason)
			}
			if sing == store.NounFeature || sing == store.NounEpic {
				if cmd.Flags().Changed("assume-question") || cmd.Flags().Changed("reason") {
					return emitFailure(cmd, "submit", &usageError{
						reason: "bad_args",
						msg:    fmt.Sprintf("--assume-question and --reason apply to submit task, not submit %s", sing),
					})
				}
				return runSubmitNode(cmd, sing, args[1], pr, cmd.Flags().Changed("pr"))
			}
			if sing != store.NounTask && (cmd.Flags().Changed("assume-question") || cmd.Flags().Changed("reason")) {
				return emitFailure(cmd, "submit", &usageError{reason: "bad_args", msg: "--assume-question and assumption --reason apply only to submit task; recovery --reason requires a feature or epic with --recover-merged"})
			}

			if err := requireReasonWithAssume(cmd, "submit", assume, reason); err != nil {
				return err
			}
			tree, err := loadTree(cmd)
			if err != nil {
				return emitFailure(cmd, "submit", err)
			}

			in := mutate.TransitionInput{Verb: "submit", AssumeQuestion: assume, Reason: reason}

			if sing == store.NounTask {
				node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounTask, Raw: args[1]})
				if err != nil {
					return emitFailure(cmd, "submit", err)
				}
				task, ok := node.(*store.Task)
				if !ok {
					return emitFailure(cmd, "submit", fmt.Errorf("cli: submit resolved %s to an unsupported node type", store.NounTask))
				}
				feature := tree.Features[task.FeatureID]
				epic := epicOfFeature(tree, feature)
				grain, source := store.TaskReviewGrain(tree.Manifest, epic, feature, task)

				if cmd.Flags().Changed("pr") {
					if grain != store.ReviewGrainTask {
						return emitFailure(cmd, "submit", &usageError{
							reason: "pr_under_coarse_grain",
							msg: fmt.Sprintf("resolved review grain is %s: --pr does not apply here, a task records no PR of its own "+
								"under a coarse grain; the %s's own submit records the PR (`tiller submit %s <key>`)", grain, grain, grain),
						})
					}
					prVal := pr
					in.PR = &prVal
				} else {
					ref, err := offlineReviewRefForTask(cmd, grain, taskGrainSourceLabel(tree, epic, feature, source), gen.Ref(tree, task))
					if err != nil {
						return err
					}
					in.ReviewRef = ref
				}
			} else {
				// A shelf node (bug, chore, or tweak): its PR or review ref is always
				// its own, so there is no grain to resolve at all
				// (docs/plans/2026-08-28-bugs-and-chores-design.md).
				if _, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: sing, Raw: args[1]}); err != nil {
					return emitFailure(cmd, "submit", err)
				}
				if cmd.Flags().Changed("pr") {
					prVal := pr
					in.PR = &prVal
				} else {
					ref, err := offlineReviewRefForTask(cmd, store.ReviewGrainTask, "", "")
					if err != nil {
						return err
					}
					in.ReviewRef = ref
				}
			}
			return runTransition(cmd, "submit", args[0], args[1],
				[]string{sing}, in)
		},
	}
	cmd.Flags().IntVar(&pr, "pr", 0, "the pull request number representing this change (required when the repo has a remote)")
	cmd.Flags().StringVar(&assume, "assume-question", "", "proceed under an open question, recording it in assumption_basis (submit task only)")
	cmd.Flags().StringVar(&reason, "reason", "", "justification for a task assumption or authorized merged aggregate recovery")
	cmd.Flags().BoolVar(&recoverMerged, "recover-merged", false, "record an explicitly authorized historical merged feature/epic review from integration")
	cmd.Flags().StringVar(&recoveryHead, "head", "", "historical reviewed commit OID (recovery only)")
	cmd.Flags().StringVar(&recoveryResult, "merge-commit", "", "exact integration result commit OID (recovery only)")
	addContributorFlag(cmd)
	return cmd
}

// epicOfFeature finds f's parent epic by its canonical key (Feature.EpicKey
// names the epic by key, not id), the small lookup views and gen each keep a
// private copy of for their own read surfaces (recapEpicByKey, heldEpicByKey);
// this is submit's own copy, needed to resolve a task's coarse review grain
// (store.ResolvedReviewGrain takes the epic itself, not its key). f may be
// nil (an orphaned task, never produced by any legal path) and returns nil.
func epicOfFeature(tree *store.Tree, f *store.Feature) *store.Epic {
	if f == nil {
		return nil
	}
	for _, e := range tree.Epics {
		if e.Key != nil && *e.Key == f.EpicKey {
			return e
		}
	}
	return nil
}

// prRequiredRefusal is the shared misuse (exit 2) both offlineReviewRef and
// offlineReviewRefForTask raise when a remote is configured and the caller
// still owes a PR: the PR is the review surface there, and a silent fallback
// would hide a wrong invocation. A task names the grain it resolved and
// where that came from (issue #146); feature, epic, and shelf submits pass an
// empty taskRef and keep the bare text.
func prRequiredRefusal(cmd *cobra.Command, taskRef, source string) error {
	msg := "this repo has a remote: submit with --pr <number> (the PR is the review surface)"
	if taskRef != "" {
		msg += fmt.Sprintf("; task %s resolves to review grain task (%s)", taskRef, source)
	}
	return emitFailure(cmd, "submit", &usageError{reason: "pr_required", msg: msg})
}

// taskGrainSourceLabel words store.TaskReviewGrain's source for the
// pr_required refusal: the started grain, or the home that resolved it.
func taskGrainSourceLabel(tree *store.Tree, e *store.Epic, f *store.Feature, source string) string {
	switch source {
	case "started":
		return "started at task grain"
	case "feature":
		if f != nil {
			return fmt.Sprintf("feature %s in this checkout", gen.Ref(tree, f))
		}
	case "epic":
		if e != nil {
			return fmt.Sprintf("epic %s in this checkout", gen.Ref(tree, e))
		}
	case "project":
		return "project setting"
	}
	return "default"
}

// gitReviewRef reads the current branch and head commit through the
// read-only vcs window and returns them as the review ref to record: the
// remote-less fallback both offlineReviewRef and offlineReviewRefForTask
// return once they have decided the fallback is legal. No network is
// touched.
func gitReviewRef(cmd *cobra.Command, root string, g *vcs.Git) (*store.ReviewRef, error) {
	branch, err := g.Branch()
	if err != nil {
		return nil, emitFailure(cmd, "submit", err)
	}
	head, err := g.CurrentCommit()
	if err != nil {
		return nil, emitFailure(cmd, "submit", err)
	}
	return &store.ReviewRef{Branch: branch, Head: head}, nil
}

// offlineReviewRef is submit feature's and submit epic's own submit-without-
// --pr arm: legal only when the repo has no git remote. A repo with a remote
// refuses as misuse (exit 2) naming --pr unconditionally: the coarse-grain
// node's own PR is always the review surface there, with no grain-aware
// exception (that exception is offlineReviewRefForTask's own, task-only).
func offlineReviewRef(cmd *cobra.Command) (*store.ReviewRef, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, emitFailure(cmd, "submit", err)
	}
	g := vcs.New(root)
	hasRemote, err := g.HasRemote()
	if err != nil {
		return nil, emitFailure(cmd, "submit", err)
	}
	if hasRemote {
		return nil, prRequiredRefusal(cmd, "", "")
	}
	return gitReviewRef(cmd, root, g)
}

// offlineReviewRefForTask is submit task's own review-ref gate (controller
// ruling C). No remote: the fallback is legal regardless of grain, exactly
// as offlineReviewRef's own no-remote arm. A remote: task grain raises the
// pr_required refusal, naming taskRef's grain and its source label (empty
// taskRef, as for a shelf node, keeps offlineReviewRef's bare text), but
// a coarse resolved grain (the task's feature or its epic) accepts the
// fallback anyway, because a coarse-grain task never carries a PR of its own
// at all, remote or not (docs/plans/2026-08-26-review-grain-design.md, line
// 37; the execute skill's own promise): the feature or epic PR is the review
// surface, and the task records a review ref instead.
func offlineReviewRefForTask(cmd *cobra.Command, grain, source, taskRef string) (*store.ReviewRef, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, emitFailure(cmd, "submit", err)
	}
	g := vcs.New(root)
	hasRemote, err := g.HasRemote()
	if err != nil {
		return nil, emitFailure(cmd, "submit", err)
	}
	if hasRemote && grain == store.ReviewGrainTask {
		return nil, prRequiredRefusal(cmd, taskRef, source)
	}
	return gitReviewRef(cmd, root, g)
}

// SubmitNodeView is the --json success payload of submit feature|epic: the
// resolved node's identity and the review surface now on record, the
// coarse-grain sibling of TransitionView carrying only the fields
// mutate.SubmitNode's own result has (no Status: the reviewed node's status
// never moves, mutate.SubmitNode's own doc comment).
type SubmitNodeView struct {
	Noun      string         `json:"noun"`
	ID        string         `json:"id"`
	Key       string         `json:"key,omitempty"`
	PR        *int           `json:"pr,omitempty"`
	ReviewRef *ReviewRefView `json:"review_ref,omitempty"`
	Changed   bool           `json:"changed"`
}

func submitNodeView(r mutate.SubmitNodeResult) SubmitNodeView {
	return SubmitNodeView{Noun: r.Noun, ID: r.ID, Key: r.Key, PR: r.PR, ReviewRef: reviewRefView(r.ReviewRef), Changed: r.Changed}
}

// runSubmitNode is submit feature's and submit epic's own shell: resolve the
// owner, build the PR-or-review-ref payload the same either-or task-grain
// submit already carries, drive mutate.SubmitNode, and render the outcome.
func runSubmitNode(cmd *cobra.Command, noun, key string, pr int, prChanged bool) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "submit", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "submit", err)
	}
	in := mutate.SubmitNodeInput{Noun: noun, Key: key}
	if prChanged {
		v := pr
		in.PR = &v
	} else {
		ref, err := offlineReviewRef(cmd)
		if err != nil {
			return err
		}
		in.ReviewRef = ref
	}
	res, err := mutate.SubmitNode(tree, d, in)
	if err != nil {
		target, _ := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: noun, Raw: key})
		return reportSubmitNodeError(cmd, tree, target, err)
	}
	data, human := withUpNext(cmd, tree.Root, submitNodeView(res), humanSubmitNode(tree, res))
	return emitSuccess(cmd, "submit", data, human)
}

// humanSubmitNode is submit feature's and submit epic's own plain-text
// confirmation: record-only, so it never says "is now <status>" the way
// humanTransition's line does (nothing moved), instead naming the review
// surface now on record.
func humanSubmitNode(tree *store.Tree, r mutate.SubmitNodeResult) string {
	ref := gen.RefByID(tree, r.ID)
	if !r.Changed {
		if r.PR != nil {
			return fmt.Sprintf("%s %s already records pr #%d as its review surface (no change).", r.Noun, ref, *r.PR)
		}
		return fmt.Sprintf("%s %s already records this review ref as its review surface (no change).", r.Noun, ref)
	}
	if r.PR != nil {
		return fmt.Sprintf("%s %s now records pr #%d as its review surface.", r.Noun, ref, *r.PR)
	}
	return fmt.Sprintf("%s %s now records branch %s at %s as its review surface; no remote, review happens on the branch.",
		r.Noun, ref, r.ReviewRef.Branch, strings.TrimPrefix(r.ReviewRef.Head.Value, "git:"))
}

// NodeNotStartedData is the BLOCKED data block for submit feature|epic's own
// not-started refusal (mutate.NodeNotStartedError): the reviewed node has
// not begun work yet.
type NodeNotStartedData struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Reason string `json:"reason"`
	Status string `json:"status"`
}

// NodeTerminalData is the BLOCKED data block for submit feature|epic's own
// terminal refusal (mutate.NodeTerminalError): the reviewed node has already
// closed.
type NodeTerminalData struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Reason string `json:"reason"`
	Status string `json:"status"`
}

// TaskNotSubmittableData is the BLOCKED data block for submit feature's own
// task-readiness refusal (mutate.TaskNotSubmittableError): a task under the
// reviewed feature is neither in_review nor done, or there is no live task
// at all (Task empty). Reason matches FeatureNotSubmittableData's own
// "tasks_incomplete", the epic-grain sibling of this exact refusal one grain
// up, so a caller branches on one reason string at either grain.
type TaskNotSubmittableData struct {
	Noun       string `json:"noun"`
	ID         string `json:"id"`
	Key        string `json:"key,omitempty"`
	Reason     string `json:"reason"`
	Task       string `json:"task,omitempty"`
	TaskKey    string `json:"task_key,omitempty"`
	TaskStatus string `json:"task_status,omitempty"`
}

// RequiredGatesNotGreenData is the BLOCKED data block for submit's gate
// refusal (mutate.RequiredGatesNotGreenError): the node's latest recorded
// result for some gate required at its grain is not a pass.
type RequiredGatesNotGreenData struct {
	Noun    string   `json:"noun"`
	ID      string   `json:"id"`
	Key     string   `json:"key,omitempty"`
	Reason  string   `json:"reason"`
	Missing []string `json:"missing"`
}

// FeatureNotSubmittableData is the BLOCKED payload for a feature that cannot
// ride the submitted epic: its tasks or gates are incomplete, or an explicit
// task/feature override has not reached done.
type FeatureNotSubmittableData struct {
	Noun       string `json:"noun"`
	ID         string `json:"id"`
	Key        string `json:"key,omitempty"`
	Reason     string `json:"reason"`
	Feature    string `json:"feature"`
	FeatureKey string `json:"feature_key,omitempty"`
	Task       string `json:"task,omitempty"`
}

// TaskReviewOpenData is the BLOCKED payload for task-grain review history
// that must merge before a coarse review can open: a task PR, or, for a
// remoteless task that started at task grain, its review_ref branch.
type TaskReviewOpenData struct {
	Noun         string `json:"noun"`
	ID           string `json:"id"`
	Key          string `json:"key,omitempty"`
	Reason       string `json:"reason"`
	Task         string `json:"task"`
	TaskKey      string `json:"task_key,omitempty"`
	PR           *int   `json:"pr,omitempty"`
	ReviewBranch string `json:"review_branch,omitempty"`
	StartedGrain string `json:"started_grain,omitempty"`
}

// reportSubmitNodeError first reuses the transition blocked renderers for the
// shared work guards, then handles coarse-submit readiness errors.
// Wrong-grain misuse and environmental/write failures remain tool
// failures.
func reportSubmitNodeError(cmd *cobra.Command, tree *store.Tree, target store.Node, err error) error {
	var questions *mutate.BlockedByQuestionsError
	if errors.As(err, &questions) {
		d, handles := blockedByQuestionsView(questions)
		human := fmt.Sprintf(`Blocked: %s %s is held by %d open question(s): %s; answer the questions, or cancel one that no longer needs a decision with tiller cancel question <Q-id> --reason "...", and retry.`,
			questions.Noun, gen.RefByID(tree, questions.NodeID), len(questions.Blockers), strings.Join(handles, ", "))
		return emitReport(cmd, "submit", false, d, human, 1)
	}
	if data, human, matched := transitionBlockedView(tree, target, err); matched {
		return emitReport(cmd, "submit", false, data, human, 1)
	}
	var taskReview *mutate.TaskReviewOpenError
	if errors.As(err, &taskReview) {
		d := TaskReviewOpenData{
			Noun: taskReview.ReviewedNoun, ID: taskReview.ReviewedID, Key: taskReview.ReviewedKey,
			Reason: mutate.ReasonTaskReviewOpen, Task: taskReview.TaskID, TaskKey: taskReview.TaskKey, PR: taskReview.PR,
			StartedGrain: taskReview.StartedGrain,
		}
		review, own := "", "PR"
		if taskReview.PR != nil {
			review = fmt.Sprintf("has PR #%d open", *taskReview.PR)
		} else {
			d.ReviewBranch = taskReview.ReviewBranch
			review, own = "has its own review open on "+taskReview.ReviewBranch, "PR or review ref"
		}
		human := fmt.Sprintf("Blocked: %s %s cannot submit while task %s %s; run `tiller sync` after it merges.",
			taskReview.ReviewedNoun, gen.RefByID(tree, taskReview.ReviewedID),
			gen.RefByID(tree, taskReview.TaskID), review)
		if taskReview.StartedGrain == store.ReviewGrainTask {
			human += fmt.Sprintf(" Task %s started at task grain, so it takes its own %s and must merge before this submit.", gen.RefByID(tree, taskReview.TaskID), own)
		}
		return emitReport(cmd, "submit", false, d, human, 1)
	}
	var notStarted *mutate.NodeNotStartedError
	if errors.As(err, &notStarted) {
		d := NodeNotStartedData{Noun: notStarted.Noun, ID: notStarted.NodeID, Key: notStarted.Key, Reason: "node_not_started", Status: notStarted.Status}
		human := fmt.Sprintf("Blocked: %s %s has nothing to review yet (%s).", notStarted.Noun, gen.RefByID(tree, notStarted.NodeID), notStarted.Status)
		return emitReport(cmd, "submit", false, d, human, 1)
	}
	var terminal *mutate.NodeTerminalError
	if errors.As(err, &terminal) {
		d := NodeTerminalData{Noun: terminal.Noun, ID: terminal.NodeID, Key: terminal.Key, Reason: "node_terminal", Status: terminal.Status}
		human := fmt.Sprintf("Blocked: %s %s is terminal (%s); submit records nothing further once a node has closed.",
			terminal.Noun, gen.RefByID(tree, terminal.NodeID), terminal.Status)
		return emitReport(cmd, "submit", false, d, human, 1)
	}
	var taskBlk *mutate.TaskNotSubmittableError
	if errors.As(err, &taskBlk) {
		d := TaskNotSubmittableData{
			Noun: store.NounFeature, ID: taskBlk.FeatureID, Key: taskBlk.FeatureKey, Reason: "tasks_incomplete",
			Task: taskBlk.TaskID, TaskKey: taskBlk.TaskKey, TaskStatus: taskBlk.TaskStatus,
		}
		human := fmt.Sprintf("Blocked: feature %s has no live task to review.", gen.RefByID(tree, taskBlk.FeatureID))
		if taskBlk.TaskID != "" {
			human = fmt.Sprintf("Blocked: feature %s cannot submit: task %s is %s, not in_review or done.",
				gen.RefByID(tree, taskBlk.FeatureID), gen.RefByID(tree, taskBlk.TaskID), taskBlk.TaskStatus)
		}
		return emitReport(cmd, "submit", false, d, human, 1)
	}
	var featBlk *mutate.FeatureNotSubmittableError
	if errors.As(err, &featBlk) {
		d := FeatureNotSubmittableData{
			Noun: store.NounEpic, ID: featBlk.EpicID, Key: featBlk.EpicKey,
			Reason: featBlk.Reason, Feature: featBlk.FeatureID, FeatureKey: featBlk.FeatureKey,
			Task: featBlk.TaskID,
		}
		// Mirrors taskBlk's own conditional branch shape immediately above:
		// the no-live-task wording is the default, overridden when TaskID
		// names an actual blocking task (micro-round fix; item 6 already
		// fixed this message at the mutate layer, mutate.FeatureNotSubmittableError.Error(),
		// but this CLI rendering kept the old unconditional wording).
		human := fmt.Sprintf("Blocked: epic %s cannot submit: feature %s has no live task to review.",
			gen.RefByID(tree, featBlk.EpicID), gen.RefByID(tree, featBlk.FeatureID))
		if featBlk.TaskID != "" {
			human = fmt.Sprintf("Blocked: epic %s cannot submit: feature %s has a task not yet in_review or done.",
				gen.RefByID(tree, featBlk.EpicID), gen.RefByID(tree, featBlk.FeatureID))
		}
		if featBlk.Reason == mutate.ReasonIndependentReviewOpen {
			human = fmt.Sprintf("Blocked: epic %s cannot submit: overridden feature %s must have its own review reach done before the epic PR opens.",
				gen.RefByID(tree, featBlk.EpicID), gen.RefByID(tree, featBlk.FeatureID))
		}
		if featBlk.Reason == "gates_not_green" {
			human = fmt.Sprintf("Blocked: epic %s cannot submit: feature %s has not recorded every required gate green; run `tiller check feature %s` first.",
				gen.RefByID(tree, featBlk.EpicID), gen.RefByID(tree, featBlk.FeatureID), gen.RefByID(tree, featBlk.FeatureID))
		}
		return emitReport(cmd, "submit", false, d, human, 1)
	}
	return emitFailure(cmd, "submit", err)
}

// requireReasonWithAssume rejects an --assume-question without a paired --reason
// as misuse (exit 2) before any work, so an assumption is never recorded without
// its justification.
func requireReasonWithAssume(cmd *cobra.Command, command, assume, reason string) error {
	if strings.TrimSpace(assume) != "" && strings.TrimSpace(reason) == "" {
		return emitFailure(cmd, command, &usageError{
			reason: "reason_required",
			msg:    "--assume-question requires a paired --reason",
		})
	}
	return nil
}

// runTransition is the shared body of every lifecycle verb: validate the noun for
// this verb, resolve the address, apply the transition through the core, and
// render the success or the recoverable blocked report.
func runTransition(cmd *cobra.Command, command, noun, node string, allowed []string, in mutate.TransitionInput) error {
	tree, n, err := resolveForMutation(cmd, command, noun, node, allowed)
	if err != nil {
		return err
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, command, err)
	}
	res, err := mutate.Transition(tree, d, n, in)
	if err != nil {
		return reportTransitionError(cmd, command, tree, n, err)
	}

	// The review-state warning (issue #42) is only ever computed for submit:
	// every other lifecycle verb's node is never left in_review with a fresh
	// PR by its own transition, and gating on the command name here means the
	// other eight verbs never pay for a git read that could not apply to
	// them.
	//
	// It is computed off a FRESH tree reload, not the tree variable already in
	// hand, because mutate.Transition never mutates the in-memory tree it was
	// given: transitionTask's own loadTaskFile (add.go) reads a brand new
	// *store.Task off disk into `target`, mutates and persists that separate
	// object, and returns; the tree.Tasks entry this function already holds a
	// pointer to is untouched and still reads the pre-transition status and a
	// nil PR. review.ForTask keyed off the stale tree found nothing to warn
	// about in every real-repo test run, which is what surfaced this.
	// renderReadoutFor (readout.go) hit the identical staleness for a gate's
	// post-mutation readout and settled the same fix: reload from root rather
	// than trust the in-memory tree. A reload failure degrades to no warning
	// rather than failing a submit that has already succeeded, the same
	// "a reporting problem must never become a transition failure" rule
	// renderReadoutFor's own doc comment states.
	var unpushed *UnpushedReviewFacts
	if command == "submit" {
		if fresh, ferr := store.Load(tree.Root); ferr == nil {
			unpushed = review.ForTask(fresh, res.ID, vcs.New(tree.Root))
		}
	}

	view := transitionView(res)
	view.UnpushedReview = unpushedReviewView(unpushed)
	human := humanTransition(tree, command, in, res, gen.Ref(tree, n), unpushed)
	var data any = view
	if command == "submit" {
		// A completing command ends on the ranked list (decision 9).
		data, human = withUpNext(cmd, tree.Root, data, human)
	}
	return emitTransitionSuccess(cmd, command, tree.Root, gateMomentFor(command),
		transitionAct(in, res), data, human)
}

// transitionView projects a transition result onto the JSON envelope, carrying
// the tasks a plan gate moved alongside the node addressed.
func transitionView(r mutate.TransitionResult) TransitionView {
	v := TransitionView{
		Noun: r.Noun, ID: r.ID, Key: r.Key, Status: r.Status, Branch: r.Branch, BaseBranch: r.BaseBranch,
		AggregateBranch: r.AggregateBranch, AdoptedCommits: r.AdoptedCommits,
		PR: r.PR, ReviewRef: reviewRefView(r.ReviewRef), Changed: r.Changed,
		Warnings: r.Warnings,
	}
	for _, tk := range r.Tasks {
		v.Tasks = append(v.Tasks, transitionView(tk))
	}
	return v
}

// transitionAct is the readout's record of what this transition did: one entry
// per node it moved. A plan gate moves the feature and its initial tasks, so all
// of them are named, and every other transition names the one node addressed.
//
// It carries in.RefreshBasis through because the result cannot: a refresh
// records the same Noun, ID, and Changed a real transition does, and the node on
// disk afterwards looks exactly as it did before. The flag the command was
// invoked with is the only thing that tells the two apart.
func transitionAct(in mutate.TransitionInput, r mutate.TransitionResult) *views.GateAct {
	act := &views.GateAct{
		Nodes:        []views.GateActNode{{Noun: r.Noun, ID: r.ID, Changed: r.Changed}},
		BasisRefresh: in.RefreshBasis,
	}
	for _, tk := range r.Tasks {
		act.Nodes = append(act.Nodes, views.GateActNode{Noun: tk.Noun, ID: tk.ID, Changed: tk.Changed})
	}
	return act
}

// resolveForMutation validates the noun against the verbs the node class allows,
// loads the tree, and resolves the address to one node. It emits the failure
// envelope on any error and returns the wrapped exitError, so a caller propagates
// a non-nil error directly. Unlike the read path it returns the tree too, because
// the mutation core needs it.
func resolveForMutation(cmd *cobra.Command, command, noun, node string, allowed []string) (*store.Tree, store.Node, error) {
	sing, err := validNoun(cmd, command, noun, allowed)
	if err != nil {
		return nil, nil, err
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return nil, nil, emitFailure(cmd, command, err)
	}
	n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: sing, Raw: node})
	if err != nil {
		return nil, nil, emitFailure(cmd, command, err)
	}
	return tree, n, nil
}

// validNoun validates a command's noun against the set the verb allows,
// emitting the misuse envelope on failure so a caller propagates the error
// directly. It returns the singular form the resolver takes.
func validNoun(cmd *cobra.Command, command, noun string, allowed []string) (string, error) {
	sing, ok := singularNoun(noun)
	if !ok || !containsNoun(allowed, sing) {
		return "", emitFailure(cmd, command, &usageError{
			reason: "unknown_noun",
			msg:    fmt.Sprintf("%s does not apply to %q (want one of %s)", command, noun, strings.Join(allowed, ", ")),
		})
	}
	return sing, nil
}

// reportTransitionError routes a transition error to the right envelope: an
// illegal move or a structural-blocker refusal is a BLOCKED business answer (exit
// 1) carrying a data block; anything else (a bad owner, a write fault) is a tool
// failure with an error block.
//
// Claim commands resolve internally and pass n=nil; their errors carry the
// execution unit's own NodeID and Key.
func reportTransitionError(cmd *cobra.Command, command string, tree *store.Tree, n store.Node, err error) error {
	data, human, matched := transitionBlockedView(tree, n, err)
	if !matched {
		return emitFailure(cmd, command, err)
	}
	return emitReport(cmd, command, false, data, human, 1)
}

// transitionBlockedView maps a typed blocked transition error onto its data
// block and human line, the single classification the single-node envelope and
// the batch per-node entries both render. matched is false for anything that is
// not a blocked business answer (a bad owner, a write fault), which the caller
// treats as a tool failure. Human lines name the blocked node by the shared
// reference convention; the blocker errors carry bare ids, so each arm resolves
// its own id through the tree (the blocked node is not always the node acted
// on: starting a task can surface its parent feature's incomplete spec).
//
// The last six arms are the owner lifecycle's blocked-answer family: the
// epic-branch owner-alignment refusal, three raised directly by claim/release,
// and two grain-neutral work-gate errors raised by feature start or task
// start/submit/resume. They share one rendering idiom even though
// claim/release reach this function without a resolved node.
func blockedByQuestionsView(blk *mutate.BlockedByQuestionsError) (BlockedByQuestionsData, []string) {
	data := BlockedByQuestionsData{
		Noun:         blk.Noun,
		ID:           blk.NodeID,
		Reason:       "open_question_blocker",
		CurrentState: blk.CurrentState,
		Blockers:     blockerViews(blk.Blockers),
	}
	handles := make([]string, len(blk.Blockers))
	for i, blocker := range blk.Blockers {
		handles[i] = questionTitleHandle(blocker.Question, blocker.Title)
	}
	return data, handles
}

func transitionBlockedView(tree *store.Tree, n store.Node, err error) (data any, human string, matched bool) {
	if data, human, matched := branchBlockedView(tree, err); matched {
		return data, human, true
	}
	var gatesBlk *mutate.RequiredGatesNotGreenError
	if errors.As(err, &gatesBlk) {
		d := RequiredGatesNotGreenData{
			Noun: gatesBlk.Noun, ID: gatesBlk.NodeID, Key: gatesBlk.Key,
			Reason: "gates_not_green", Missing: nonNilStrings(gatesBlk.Missing),
		}
		addr := gatesBlk.Key
		if addr == "" {
			addr = gatesBlk.NodeID
		}
		return d, fmt.Sprintf("Blocked: %s %s has not recorded every required gate green (%s); run `tiller check %s %s` first.",
			gatesBlk.Noun, gen.RefByID(tree, gatesBlk.NodeID), strings.Join(gatesBlk.Missing, ", "), gatesBlk.Noun, addr), true
	}

	var ill *lifecycle.IllegalTransitionError
	if errors.As(err, &ill) {
		d := IllegalTransitionData{
			Noun:         ill.Noun,
			ID:           n.NodeID(),
			Key:          keyOf(n),
			Reason:       "illegal_transition",
			Verb:         ill.Verb,
			CurrentState: ill.From,
			LegalNext:    nonNilStrings(ill.Legal),
		}
		return d, fmt.Sprintf("Blocked: cannot %s %s %s from %q. Legal next: %s.",
			ill.Verb, ill.Noun, gen.Ref(tree, n), ill.From, strings.Join(d.LegalNext, ", ")), true
	}

	var blk *mutate.BlockedByQuestionsError
	if errors.As(err, &blk) {
		d, handles := blockedByQuestionsView(blk)
		// A plan gate (approve) never offers --assume-question: a feature's
		// assumption does not reach its tasks, so the way out is to answer
		// the question and fold the answer into the plan (issue #152,
		// decision 8). The literal <Q-id> is a placeholder; the handles
		// already name each question's id and title.
		remedy := `answer them, cancel one that no longer needs a decision with tiller cancel question <Q-id> --reason "...", or pass --assume-question.`
		if blk.Verb == "approve" {
			remedy = `answer them with tiller answer question <Q-id> --answer "...", or cancel one that no longer needs a decision with tiller cancel question <Q-id> --reason "...", fold the answers into the plan, and retry.`
		}
		return d, fmt.Sprintf("Blocked: %s %s is held by %d open question(s): %s; %s",
			blk.Noun, gen.RefByID(tree, blk.NodeID), len(blk.Blockers), strings.Join(handles, ", "), remedy), true
	}

	var inc *mutate.SpecIncompleteError
	if errors.As(err, &inc) {
		d := SpecIncompleteData{
			Noun:         store.NounFeature,
			ID:           inc.FeatureID,
			Reason:       "spec_incomplete",
			CurrentState: inc.CurrentState,
			Incomplete:   nonNilStrings(inc.Incomplete),
		}
		state := "is not specced"
		if inc.CurrentState == "specced" {
			// Re-spec and the plan gate re-validate a specced feature (#145).
			state = "is specced, but its specify output no longer passes"
		}
		return d, fmt.Sprintf("Blocked: feature %s %s; write %s%s (run `tiller guide specify` for the walkthrough).",
			gen.RefByID(tree, inc.FeatureID), state, strings.Join(inc.Incomplete, ", "),
			specIncompleteDirClause(tree, inc.FeatureID)), true
	}

	var pg *mutate.ParentPlanUngatedError
	if errors.As(err, &pg) {
		seam := "tiller approve feature " + pg.FeatureKey
		human := fmt.Sprintf("Blocked: task %s runs under feature %s, specced but not approved; the plan gate is `%s`.",
			gen.RefByID(tree, pg.NodeID), gen.RefByID(tree, pg.FeatureID), seam)
		if pg.FeatureState == "draft" {
			seam = "tiller spec feature " + pg.FeatureKey
			human = fmt.Sprintf("Blocked: task %s runs under feature %s, still draft; write and gate its plan first (run `tiller guide specify` for the walkthrough).",
				gen.RefByID(tree, pg.NodeID), gen.RefByID(tree, pg.FeatureID))
		}
		d := ParentPlanUngatedData{
			Noun:         store.NounTask,
			ID:           pg.NodeID,
			Reason:       "parent_plan_ungated",
			CurrentState: pg.CurrentState,
			Feature:      pg.FeatureID,
			FeatureKey:   pg.FeatureKey,
			FeatureState: pg.FeatureState,
			MissingSeam:  seam,
		}
		return d, human, true
	}

	// The cancelled arm is a different answer from the ungated one above, not a
	// third state on it (mutate.ParentCancelledError documents why): cancel is
	// legal from draft, specced, ready, and active, so a cancelled parent may
	// carry a fully approved plan. What the two arms share is the refusal; what
	// differs is the remedy. A cancelled feature is terminal, so the remedy
	// names the way out instead of a next gate: retire the task too, or re-add
	// the feature and plan the work anew. The seam names the task by its bare
	// id, not its key: gen.RefByID renders a display ref ("key (id)"), which is
	// not a valid command argument, and the file offers no id-to-key helper for
	// a render site holding only an id, so the id alone is the honest minimal
	// choice for a copy-pasteable command (issue #35).
	var pc *mutate.ParentCancelledError
	if errors.As(err, &pc) {
		seam := "tiller cancel task " + pc.NodeID + " --reason \"<why>\""
		d := ParentCancelledData{
			Noun:         store.NounTask,
			ID:           pc.NodeID,
			Reason:       "parent_feature_cancelled",
			CurrentState: pc.CurrentState,
			Feature:      pc.FeatureID,
			FeatureKey:   pc.FeatureKey,
			MissingSeam:  seam,
		}
		return d, fmt.Sprintf("Blocked: task %s runs under feature %s, which is cancelled; cancel the task too (`%s`), or re-add the feature and plan the work anew.",
			gen.RefByID(tree, pc.NodeID), gen.RefByID(tree, pc.FeatureID), seam), true
	}

	// The epic arm answers a grandparent question, not the parent one the
	// cancelled arm above answers (mutate.ParentEpicRetiredError documents why:
	// cancel epic is legal from ready while a feature beneath it is still
	// active, so a live feature can sit under a retired epic). It fires at
	// either grain, feature or task, so unlike the cancelled arm above it reads
	// its noun from the error rather than hardcoding one, and the seam and human
	// line both name the refused node by that same noun.
	var pe *mutate.ParentEpicRetiredError
	if errors.As(err, &pe) {
		seam := "tiller cancel " + pe.Noun + " " + pe.NodeID + " --reason \"<why>\""
		d := ParentEpicRetiredData{
			Noun:         pe.Noun,
			ID:           pe.NodeID,
			Reason:       "parent_epic_retired",
			CurrentState: pe.CurrentState,
			Epic:         pe.EpicID,
			EpicKey:      pe.EpicKey,
			EpicState:    pe.EpicState,
			MissingSeam:  seam,
		}
		return d, fmt.Sprintf("Blocked: %s %s runs under epic %s, which is %s; cancel the %s too (`%s`), or re-add the epic and plan the work anew.",
			pe.Noun, gen.RefByID(tree, pe.NodeID), gen.RefByID(tree, pe.EpicID), pe.EpicState, pe.Noun, seam), true
	}

	var ab *mutate.ActiveBasisError
	if errors.As(err, &ab) {
		d := ActiveBasisData{
			Noun:         ab.Noun,
			ID:           ab.NodeID,
			Reason:       "active_basis",
			CurrentState: ab.CurrentState,
			Drifted:      nonNilStrings(ab.Drifted),
			Missing:      ab.Missing,
		}
		return d, "Blocked: " + activeBasisBlockedLine(tree, n, ab), true
	}

	var aq *mutate.AnsweredQuestionError
	if errors.As(err, &aq) {
		d := AnsweredQuestionData{
			Noun:      aq.Noun,
			ID:        aq.NodeID,
			Reason:    "answered_question",
			Questions: answeredQuestionViews(aq.Questions),
			Remedy:    answeredQuestionRemedy(tree, aq),
		}
		return d, "Blocked: " + answeredQuestionBlockedLine(tree, n, aq), true
	}

	var grainBlocked *mutate.ReviewGrainChangeBlockedError
	if errors.As(err, &grainBlocked) {
		data, human := reviewGrainChangeBlockedView(tree, grainBlocked)
		return data, human, true
	}

	var ch *mutate.ClaimHeldError
	if errors.As(err, &ch) {
		d := ClaimHeldData{
			Noun: ch.Noun, ID: ch.NodeID, Key: ch.Key,
			Reason: ch.Reason, Holder: ch.Holder, Seam: ch.Seam,
		}
		return d, fmt.Sprintf("Blocked: %s %s is claimed by %s: re-run with `%s` to take it.",
			ch.Noun, gen.RefByID(tree, ch.NodeID), ch.Holder, ch.Seam), true
	}

	var chm *mutate.ClaimHolderMismatchError
	if errors.As(err, &chm) {
		d := ClaimHolderMismatchData{
			Noun: chm.Noun, ID: chm.NodeID, Key: chm.Key,
			Reason: chm.Reason, Holder: chm.Holder, Seam: chm.Seam,
		}
		if chm.Holder == "" {
			return d, fmt.Sprintf("Blocked: %s %s is unclaimed; --from names no holder to take over from.",
				chm.Noun, gen.RefByID(tree, chm.NodeID)), true
		}
		return d, fmt.Sprintf("Blocked: %s %s is held by %s, not the --from you named; re-run with `%s` to take it.",
			chm.Noun, gen.RefByID(tree, chm.NodeID), chm.Holder, chm.Seam), true
	}

	var nc *mutate.NotClaimantError
	if errors.As(err, &nc) {
		d := NotClaimantData{
			Noun: nc.Noun, ID: nc.NodeID, Key: nc.Key,
			Reason: nc.Reason, Holder: nc.Holder, Seam: nc.Seam,
		}
		return d, fmt.Sprintf("Blocked: %s %s is claimed by %s, not you; reassign it (`%s`).",
			nc.Noun, gen.RefByID(tree, nc.NodeID), nc.Holder, nc.Seam), true
	}

	var immutable *mutate.OwnerImmutableError
	if errors.As(err, &immutable) {
		d := map[string]any{"noun": immutable.Noun, "id": immutable.NodeID, "key": immutable.Key, "reason": immutable.Reason, "current_state": immutable.CurrentState, "owner": immutable.Owner}
		human := immutable.Error()
		if immutable.Seam != "" {
			d["seam"] = immutable.Seam
			human += "; a human-directed continuation uses `" + immutable.Seam + "`"
		}
		return d, "Blocked: " + human, true
	}
	var wu *mutate.WorkUnclaimedError
	if errors.As(err, &wu) {
		d := WorkUnclaimedData{
			Noun: wu.Noun, ID: wu.NodeID, Key: wu.Key, Reason: wu.Reason, CurrentState: wu.CurrentState, Seam: wu.Seam,
		}
		return d, fmt.Sprintf("Blocked: %s %s is unclaimed; claim it first (`%s`).",
			wu.Noun, gen.RefByID(tree, wu.NodeID), wu.Seam), true
	}

	var wc *mutate.WorkClaimHeldError
	if errors.As(err, &wc) {
		d := WorkClaimHeldData{
			Noun: wc.Noun, ID: wc.NodeID, Key: wc.Key, Reason: wc.Reason, CurrentState: wc.CurrentState,
			Holder: wc.Holder, Seam: wc.Seam, ReassignSeam: wc.ReassignSeam,
		}
		return d, fmt.Sprintf("Blocked: %s %s belongs to %s; coordinate with that owner. An explicit human handoff may use `%s`.",
			wc.Noun, gen.RefByID(tree, wc.NodeID), wc.Holder, wc.Seam), true
	}

	return nil, "", false
}

// specIncompleteDirClause names the folder the unwritten spec files live in,
// as " under <dir>/". The Incomplete entries are relative to the feature's
// folder (mutate.validateSpec), and a dry run bounced between spec and approve
// because the refusal named files with no home (JC, 2026-07-30). The folder
// comes from gen.ProsePath, the same resolution every other prose reader uses;
// a feature that cannot resolve one (keyless, which no feature at this gate
// should be) drops the clause rather than inventing a path.
func specIncompleteDirClause(tree *store.Tree, featureID string) string {
	f, ok := tree.Features[featureID]
	if !ok {
		return ""
	}
	prose, ok := gen.ProsePath(tree, f)
	if !ok {
		return ""
	}
	return " under " + path.Dir(prose) + "/"
}

// activeBasisBlockedLine is the refusal sentence for a drifted basis. n is the
// node the caller addressed and ab names the node whose pin actually moved,
// which is not the same node when a task is refused on its PARENT feature's
// spec or contract (mutate.guardTask compares that pair first).
//
// The two-node case is written as two clauses because the refusal is answering
// "why can this node not move", and the caller's own node is what they asked
// about. Naming only the feature left `tiller start task <id>` answering with a
// sentence that never mentions the task (task-9-findings-round-1.md, small item
// 5), which reads as a refusal of some other command. The remedy still addresses
// the feature, because that is the pin that has to be refreshed.
func activeBasisBlockedLine(tree *store.Tree, n store.Node, ab *mutate.ActiveBasisError) string {
	if ab.NodeID == n.NodeID() {
		return fmt.Sprintf("%s %s: %s; %s.",
			ab.Noun, gen.RefByID(tree, ab.NodeID), activeBasisDrifted(ab), activeBasisRemedy(tree, ab))
	}
	return fmt.Sprintf("%s %s is held by its %s: %s on %s %s; %s.",
		n.Noun(), gen.Ref(tree, n), ab.Noun, activeBasisDrifted(ab),
		ab.Noun, gen.RefByID(tree, ab.NodeID), activeBasisRemedy(tree, ab))
}

// activeBasisDrifted is the drift clause: which pinned files no longer match
// what the human approved. It says "no longer match" rather than "changed"
// because a pinned file that is gone is drift too (mutate.checkBasis), and it
// did not change, it went away. The verb agrees with the count because a feature
// pins two files and both can move in one edit.
func activeBasisDrifted(ab *mutate.ActiveBasisError) string {
	verb := "no longer match"
	if len(ab.Drifted) == 1 {
		verb = "no longer matches"
	}
	return fmt.Sprintf("%s %s the accepted basis", strings.Join(ab.Drifted, ", "), verb)
}

// activeBasisRemedy is the move a reader is handed when a node's accepted basis
// has moved. Both surfaces of the rule render it, the refusal above (start,
// check, submit) and the `status` oracle's report (status.go), so the two can
// never hand out different remedies for the same drift.
//
// It names the address, which the older refusal did not: that one read "run
// approve feature --refresh-basis", a command with no node in it, so it was not
// runnable as printed.
//
// It branches on ab.Missing because --refresh-basis re-records the pin from the
// working tree: on a file that is gone there is nothing to hash, so the command
// fails instead of clearing the drift, and a message naming it alone walks the
// reader into that failure (task-9-brief.md, inherited item 5). Restoring the
// file first is what makes the named move work.
//
// The command itself comes from views.ActiveBasisOf rather than being composed
// here, because the console's node page reports the same drift and hands out the
// same move (ui-surface.md, "Node detail"): one composer, so a terminal reader
// and a console reader cannot be told to run different things.
func activeBasisRemedy(tree *store.Tree, ab *mutate.ActiveBasisError) string {
	refresh := views.ActiveBasisOf(tree, ab).Command
	if len(ab.Missing) > 0 {
		return fmt.Sprintf("restore %s, then run `%s` after reviewing", strings.Join(ab.Missing, ", "), refresh)
	}
	return fmt.Sprintf("run `%s` after reviewing", refresh)
}

// answeredQuestionViews projects the predicate's own output onto the JSON
// view, retaining the full review context beside its short scan handle.
func answeredQuestionViews(qs []mutate.UnreviewedAssumption) []AnsweredQuestionView {
	out := make([]AnsweredQuestionView, len(qs))
	for i, q := range qs {
		out[i] = AnsweredQuestionView{ID: q.QuestionID, Title: q.Title, Statement: q.Statement}
	}
	return out
}

// questionTitleHandle is the human question handle for blocker and review
// summaries. Title has already come through gen.QuestionTitle in the core
// projection; this function only formats it and never re-derives from Statement.
func questionTitleHandle(id, title string) string {
	if title == "" {
		return id
	}
	return fmt.Sprintf("%s (%q)", id, title)
}

// answeredQuestionHandles is the comma-joined id-plus-title review context.
func answeredQuestionHandles(qs []mutate.UnreviewedAssumption) string {
	handles := make([]string, len(qs))
	for i, q := range qs {
		handles[i] = questionTitleHandle(q.QuestionID, q.Title)
	}
	return strings.Join(handles, ", ")
}

// answeredQuestionAddr is the address the reviewed-assumption remedy names:
// the key of the feature that carries the stale entry, falling back to its
// id. AssumptionBasis lives on Feature alone (task-5-brief.md's settled
// fact), so aq.NodeID always resolves through tree.Features, unlike
// basisAddr's ActiveBasisError sibling, which must also handle a task grain.
func answeredQuestionAddr(t *store.Tree, aq *mutate.AnsweredQuestionError) string {
	if f, ok := t.Features[aq.NodeID]; ok {
		if k := views.Key(f); k != "" {
			return k
		}
	}
	return aq.NodeID
}

// answeredQuestionRemedy is the exact runnable command(s) that clear every
// question the refusal names, addressing the feature that carries each entry,
// never the task submit was run on (task-6-brief.md, resolution 3).
// --reviewed-assumption reviews one question at a time by deliberate design
// (mutate.ReviewAssumption's own doc comment: a dedicated act, never a
// batch), so more than one stale entry means more than one command, joined
// into the single remedy string the data block carries.
func answeredQuestionRemedy(t *store.Tree, aq *mutate.AnsweredQuestionError) string {
	addr := answeredQuestionAddr(t, aq)
	cmds := make([]string, len(aq.Questions))
	for i, q := range aq.Questions {
		// One composer per command (views.ReviewedAssumptionCommand), shared
		// with the drift panel's candidate rows, the same rule
		// activeBasisRemedy already follows through views.ActiveBasisOf.
		cmds[i] = views.ReviewedAssumptionCommand(addr, q.QuestionID)
	}
	return strings.Join(cmds, "; ")
}

// answeredQuestionBlockedLine renders the node whose submit was requested and
// the feature carrying the stale assumption. They are the same node for
// submit feature and distinct for submit task or submit epic.
func answeredQuestionBlockedLine(tree *store.Tree, n store.Node, aq *mutate.AnsweredQuestionError) string {
	noun := "assumption"
	if len(aq.Questions) > 1 {
		noun = "assumptions"
	}
	return fmt.Sprintf("%s %s is held by %s %s's answered, unreviewed %s %s: run `%s` after reviewing.",
		n.Noun(), gen.Ref(tree, n), aq.Noun, gen.RefByID(tree, aq.NodeID),
		noun, answeredQuestionHandles(aq.Questions), answeredQuestionRemedy(tree, aq))
}

// blockerViews projects the core's question blockers onto the JSON view.
func blockerViews(bs []gen.QuestionBlocker) []QuestionBlockerView {
	out := make([]QuestionBlockerView, len(bs))
	for i, b := range bs {
		out[i] = QuestionBlockerView{
			Question: b.Question, Title: b.Title, Statement: b.Statement, Blocks: b.Blocks,
			TargetID: b.TargetID, TargetNoun: b.TargetNoun,
		}
	}
	return out
}

// humanTransition is the plain-text confirmation for a terminal reader. The
// node is named by the shared reference convention; ref is the caller-computed
// "<key-path> (<id>)". The feature start that moves a node
// into active adds the working branch it began work on, the branch sync later
// observes the merge by, so the agent does not have to open feature.yml to find
// it; the core leaves Branch empty on every other write of that same doc. A
// remote-less submit adds what it recorded (the branch and short head sha) and
// that review happens on the branch, since there is no PR to point at.
//
// The --refresh-basis arm gets its own line, because "is now <status>" describes
// a move and that arm makes none: it re-records accepted_against and returns
// ahead of the legality check, which is what lets it run on a node in progress,
// in review, or done (cli-surface.md, "Specifying a feature"). On a done task
// the old line read "task ... is now done", a crossing this command has no part
// in (task-9-brief.md, inherited item 8). The status is still named, as where
// the node stands rather than where it went.
//
// unpushed carries submit's own review-state warning (issue #42) computed at
// runTransition's call site; every other verb passes nil, and the RefreshBasis
// and no-change arms return before reaching it because neither is a shape
// submit's own transition ever produces (submit is never idempotent-safe the
// way a re-run --refresh-basis is).
//
// The result's Warnings follow whichever line was chosen, one `Warning: <text>`
// line each (the form check uses), so the no-change re-spec shows them too.
func humanTransition(tree *store.Tree, command string, in mutate.TransitionInput, r mutate.TransitionResult, ref string, unpushed *UnpushedReviewFacts) string {
	line := humanTransitionLine(tree, command, in, r, ref, unpushed)
	for _, w := range r.Warnings {
		line += "\nWarning: " + w
	}
	return line
}

func humanTransitionLine(tree *store.Tree, command string, in mutate.TransitionInput, r mutate.TransitionResult, ref string, unpushed *UnpushedReviewFacts) string {
	if in.RefreshBasis {
		return fmt.Sprintf("%s %s accepted basis re-recorded; state unchanged at %s.", r.Noun, ref, r.Status)
	}
	if !r.Changed {
		return fmt.Sprintf("%s %s is already %s (no change).", r.Noun, ref, r.Status)
	}
	line := fmt.Sprintf("%s %s is now %s.", r.Noun, ref, r.Status)
	if n := len(r.Tasks); n > 0 {
		line += fmt.Sprintf(" %d task(s) in the plan moved to ready with it.", n)
	}
	if command == "start" && r.Branch != "" {
		line += fmt.Sprintf(" Work happens on branch %s.", r.Branch)
		if r.BaseBranch != "" {
			line += fmt.Sprintf(" Create it from %s; Tiller has not changed Git refs.", r.BaseBranch)
		}
	}
	if command == "start" && r.AggregateBranch != "" {
		line += fmt.Sprintf(" Assemble task branches on %s for the aggregate review.", r.AggregateBranch)
		if in.AdoptAggregate && len(r.AdoptedCommits) == 0 {
			line += fmt.Sprintf(" Adopted existing aggregate %s; it holds nothing beyond integration.", r.AggregateBranch)
		} else if in.AdoptAggregate {
			line += fmt.Sprintf(" Adopted existing aggregate %s; it holds %d commit(s) beyond integration:", r.AggregateBranch, len(r.AdoptedCommits))
			for _, commit := range r.AdoptedCommits {
				line += "\n  " + commit
			}
		}
	}
	if command == "submit" && r.ReviewRef != nil {
		line += fmt.Sprintf(" Recorded branch %s at %s. %s",
			r.ReviewRef.Branch, strings.TrimPrefix(r.ReviewRef.Head.Value, "git:"), submittedReviewDescription(tree, r))
	}
	if unpushed != nil {
		line += "\n" + unpushedReviewWarning(unpushed.Commit, unpushed.Branch)
	}
	return line
}

func submittedReviewDescription(tree *store.Tree, result mutate.TransitionResult) string {
	if task := tree.Tasks[result.ID]; task != nil {
		if feature := tree.Features[task.FeatureID]; feature != nil {
			var epic *store.Epic
			for _, candidate := range tree.Epics {
				if candidate.Key != nil && *candidate.Key == feature.EpicKey {
					epic = candidate
					break
				}
			}
			grain, _ := store.TaskReviewGrain(tree.Manifest, epic, feature, task)
			switch grain {
			case store.ReviewGrainFeature:
				return "This task rides the feature review for " + gen.Ref(tree, feature) + "; this task reference does not establish publication or formal feature submission."
			case store.ReviewGrainEpic:
				if epic != nil {
					return "This task rides the epic review for " + gen.Ref(tree, epic) + "; this task reference does not establish publication or formal epic submission."
				}
			}
		}
	}
	return "Offline review happens on the recorded branch."
}

// containsNoun reports whether noun is in the allowed set for a verb.
func containsNoun(allowed []string, noun string) bool {
	for _, a := range allowed {
		if a == noun {
			return true
		}
	}
	return false
}

// nonNilStrings returns s, or an empty slice when s is nil, so a JSON array field
// renders as [] rather than null.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
