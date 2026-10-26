package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

// configureTargets are the three legal `configure` targets, in the order the
// design doc names them (docs/spec/cli-surface.md): the project default, an
// epic, and a feature. A bare `tiller configure --review-grain <g>` refuses
// rather than defaulting to the widest scope, and this is the exact list its
// refusal names.
var configureTargets = []string{"project", store.NounEpic, store.NounFeature}

// newConfigureCmd builds `configure <project|epic|feature> [key] --review-
// grain <task|feature|epic>`, the CLI's own writer of review_grain outside
// the approve verbs' own flag (mutate.Configure's doc comment). Authorization
// is the caller's: this command trusts it was run on an explicit instruction
// naming the target and the grain, validates the value, records the event,
// and reads back the resolved posture; it does not itself ask permission.
func newConfigureCmd() *cobra.Command {
	var reviewGrain string
	cmd := &cobra.Command{
		Use:   "configure <project|epic|feature> [key] --review-grain <task|feature|epic>",
		Short: "Record the review grain at the project, epic, or feature home.",
		Long: "configure records review_grain, the setting that names the grain a\n" +
			"human review happens at: task (the default), feature, or epic. It has\n" +
			"three targets, named the way approve names its node: `configure\n" +
			"project`, `configure epic <key>`, `configure feature <key>`. A bare\n" +
			"`configure --review-grain <g>` refuses, naming the three targets,\n" +
			"because the omitted-target reading would be the widest possible scope\n" +
			"and this command refuses rather than guesses. Every set is an event.",
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return emitFailure(cmd, "configure", &usageError{
					reason: "missing_target",
					msg: "configure needs a target: run `configure project`, `configure epic <key>`, " +
						"or `configure feature <key>`",
				})
			}
			if !cmd.Flags().Changed("review-grain") || strings.TrimSpace(reviewGrain) == "" {
				return emitFailure(cmd, "configure", &usageError{
					reason: "bad_args",
					msg:    "configure needs --review-grain <task|feature|epic>",
				})
			}

			target := args[0]
			var key string
			switch target {
			case "project":
				if len(args) > 1 {
					return emitFailure(cmd, "configure", &usageError{
						reason: "bad_args",
						msg:    "configure project takes no key",
					})
				}
			case store.NounEpic, store.NounFeature:
				if len(args) != 2 {
					return emitFailure(cmd, "configure", &usageError{
						reason: "bad_args",
						msg:    fmt.Sprintf("configure %s needs a key", target),
					})
				}
				key = args[1]
			default:
				return emitFailure(cmd, "configure", &usageError{
					reason: "unknown_target",
					msg: fmt.Sprintf("configure does not apply to %q (want one of %s)",
						target, strings.Join(configureTargets, ", ")),
				})
			}
			return runConfigure(cmd, target, key, reviewGrain)
		},
	}
	cmd.Flags().StringVar(&reviewGrain, "review-grain", "", "the review grain to record (task, feature, or epic)")
	addContributorFlag(cmd)
	return cmd
}

// ConfigureView is the --json success payload: the resolved target's
// identity (empty for project, which has none) and the grain now on record.
type ConfigureView struct {
	Noun        string `json:"noun"`
	ID          string `json:"id,omitempty"`
	Key         string `json:"key,omitempty"`
	ReviewGrain string `json:"review_grain"`
	Changed     bool   `json:"changed"`
}

// runConfigure loads the tree, resolves the acting contributor, and drives
// mutate.Configure, rendering either the success readback or the locked-grain
// blocked answer.
func runConfigure(cmd *cobra.Command, target, key, grain string) error {
	if target == "project" {
		root, err := repoRoot()
		if err != nil {
			return emitFailure(cmd, "configure", err)
		}
		manifestPath := filepath.Join(root, ".tiller", "tiller.yml")
		manifestData, err := store.ReadRepositoryFile(root, filepath.Join(".tiller", "tiller.yml"))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				err = &store.EnvError{
					Code:   3,
					Reason: store.ReasonNotARepo,
					Msg:    fmt.Sprintf("not a Tiller repo: %s not found", manifestPath),
				}
			}
			return emitFailure(cmd, "configure", err)
		}
		if _, err := store.RenderExecutionReviewGrain(manifestData, grain); err != nil {
			return emitFailure(cmd, "configure", &usageError{
				reason: "unsupported_manifest_shape",
				msg:    "mutate: " + err.Error(),
			})
		}
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "configure", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "configure", err)
	}
	res, err := mutate.Configure(tree, d, mutate.ConfigureInput{Noun: target, Key: key, ReviewGrain: grain})
	if err != nil {
		return reportConfigureError(cmd, tree, err)
	}
	view := ConfigureView{Noun: res.Noun, ID: res.ID, Key: res.Key, ReviewGrain: res.ReviewGrain, Changed: res.Changed}
	return emitSuccess(cmd, "configure", view, humanConfigure(tree, res))
}

// reviewGrainSourceLabel names where a just-recorded grain took effect: the
// project's own default, or an explicit setting at the noun named. Configure
// only ever writes at the exact home it targeted, and that home's own
// explicit value always wins ResolvedReviewGrain's own precedence
// (store.ResolvedReviewGrain), so the label is a static fact about which home
// this call wrote, not a second inheritance computation.
func reviewGrainSourceLabel(noun string) string {
	if noun == "project" {
		return "project default"
	}
	return noun + " explicit"
}

// humanConfigure composes configure's plain-text confirmation: the primary
// "review grain: <g> recorded on <ref>" line (design doc, "records the
// event"), then the resolved-posture readback ("reads back the resolved
// posture"), and a **Next** block naming the node's own remaining gate when
// one is still ahead of it (approve, for a draft epic or an unapproved
// feature plan): configure never moves a node's status itself, so if the
// human's next move is still to approve the plan, this says so. A grain that
// actually changed also says to commit it to integration before starting tasks.
func humanConfigure(tree *store.Tree, res mutate.ConfigureResult) string {
	var b strings.Builder
	if res.Noun == "project" {
		if res.Changed {
			fmt.Fprintf(&b, "review grain: %s recorded for the project.\n", res.ReviewGrain)
		} else {
			fmt.Fprintf(&b, "review grain: %s is already recorded for the project (no change).\n", res.ReviewGrain)
		}
	} else {
		ref := gen.RefByID(tree, res.ID)
		if res.Changed {
			fmt.Fprintf(&b, "review grain: %s recorded on %s.\n", res.ReviewGrain, ref)
		} else {
			fmt.Fprintf(&b, "review grain: %s is already recorded on %s (no change).\n", res.ReviewGrain, ref)
		}
	}
	fmt.Fprintf(&b, "resolved grain here is now: %s (%s).", res.ReviewGrain, reviewGrainSourceLabel(res.Noun))
	if res.Changed {
		b.WriteString("\nCommit this change to integration before starting tasks: work started in another checkout or worktree resolves the grain it can see.")
	}
	if next := configureNextGate(tree, res); next != "" {
		b.WriteString("\n\n**Next**\n")
		fmt.Fprintf(&b, "  %s\n", next)
	}
	return b.String()
}

// configureNextGate names the node's own remaining human gate, or "" when
// none follows: configure is record-only (mutate.Configure's own doc
// comment) and moves no status, so a node it just touched may still be
// sitting ahead of its own approve. It reads lifecycle.NextMoves off the
// tree's own cached copy of the node, which configure never mutates (it
// writes only review_grain), so the status read stays accurate even though
// that same cached copy's ReviewGrain field is now stale.
func configureNextGate(tree *store.Tree, res mutate.ConfigureResult) string {
	addr := res.Key
	if addr == "" {
		addr = res.ID
	}
	switch res.Noun {
	case store.NounEpic:
		e := tree.Epics[res.ID]
		if e == nil {
			return ""
		}
		if hasMove(lifecycle.NextMoves(e), "approve") {
			return "tiller approve epic " + addr
		}
	case store.NounFeature:
		f := tree.Features[res.ID]
		if f == nil {
			return ""
		}
		moves := lifecycle.NextMoves(f)
		if hasMove(moves, "spec") {
			return "tiller spec feature " + addr
		}
		if hasMove(moves, "approve") {
			// An open question holding the plan refuses the approve (issue
			// #152), so name each answer instead of a command
			// that would exit 1 when copied.
			if blockers := gen.PlanQuestionBlockers(tree, f); len(blockers) > 0 {
				answers := make([]string, len(blockers))
				for i, b := range blockers {
					answers[i] = fmt.Sprintf("tiller answer question %s --answer \"...\"", b.Question)
				}
				return strings.Join(answers, "\n  ")
			}
			return "tiller approve feature " + addr
		}
	}
	return ""
}

// hasMove reports whether verb is among moves, the tiny membership test
// configureNextGate needs over lifecycle.NextMoves' own slice.
func hasMove(moves []string, verb string) bool {
	for _, m := range moves {
		if m == verb {
			return true
		}
	}
	return false
}

// ConfigureLockedData is the BLOCKED data block for configure's own
// Changeability refusal (mutate.ReviewGrainLockedError): the node already
// carries its own review surface, or has closed, so its grain can no longer
// change.
type ConfigureLockedData struct {
	Noun   string         `json:"noun"`
	ID     string         `json:"id"`
	Key    string         `json:"key,omitempty"`
	Reason string         `json:"reason"`
	Status string         `json:"status,omitempty"`
	PR     *int           `json:"pr,omitempty"`
	Ref    *ReviewRefView `json:"review_ref,omitempty"`
}

// ReviewGrainChangeBlockedData is the shared configure/approve payload for a
// proposed grain change that would retarget live work.
type ReviewGrainChangeBlockedData struct {
	Noun          string `json:"noun"`
	ID            string `json:"id,omitempty"`
	Key           string `json:"key,omitempty"`
	Reason        string `json:"reason"`
	Desired       string `json:"desired"`
	BlockerNoun   string `json:"blocker_noun"`
	BlockerID     string `json:"blocker_id"`
	BlockerKey    string `json:"blocker_key,omitempty"`
	BlockerStatus string `json:"blocker_status"`
	BeforeGrain   string `json:"before_grain"`
	AfterGrain    string `json:"after_grain"`
}

func reviewGrainChangeBlockedView(tree *store.Tree, e *mutate.ReviewGrainChangeBlockedError) (ReviewGrainChangeBlockedData, string) {
	data := ReviewGrainChangeBlockedData{
		Noun: e.TargetNoun, ID: e.TargetID, Key: e.TargetKey, Reason: e.Reason, Desired: e.Desired,
		BlockerNoun: e.BlockerNoun, BlockerID: e.BlockerID, BlockerKey: e.BlockerKey,
		BlockerStatus: e.BlockerStatus, BeforeGrain: e.BeforeGrain, AfterGrain: e.AfterGrain,
	}
	targetRef := e.TargetNoun
	if e.TargetID != "" {
		targetRef = gen.RefByID(tree, e.TargetID)
	}
	blockerRef := gen.RefByID(tree, e.BlockerID)
	if e.Reason == mutate.ReasonReviewGrainBranchLocked {
		return data, fmt.Sprintf(
			"Blocked: changing review grain on %s would move %s into or out of the epic branch after work began; choose the grain before the first task starts.",
			targetRef, blockerRef)
	}
	return data, fmt.Sprintf(
		"Blocked: changing review grain on %s would retarget %s, currently %s; finish and sync that review first.",
		targetRef, blockerRef, e.BlockerStatus)
}

// reportConfigureError classifies a mutate.Configure error: the locked-grain
// refusal is a blocked business answer (exit 1, ok:false, a data block),
// exactly like the transition family's own blocked answers
// (transitionBlockedView); anything else (a bad owner, an invalid grain
// value, an unsupported noun) is a tool-level failure.
func reportConfigureError(cmd *cobra.Command, tree *store.Tree, err error) error {
	var locked *mutate.ReviewGrainLockedError
	if errors.As(err, &locked) {
		d := ConfigureLockedData{
			Noun: locked.Noun, ID: locked.ID, Key: locked.Key,
			Reason: "review_grain_locked", Status: locked.Status,
			PR: locked.PR, Ref: reviewRefView(locked.ReviewRef),
		}
		return emitReport(cmd, "configure", false, d, "Blocked: "+configureLockedLine(tree, locked), 1)
	}
	var blocked *mutate.ReviewGrainChangeBlockedError
	if errors.As(err, &blocked) {
		data, human := reviewGrainChangeBlockedView(tree, blocked)
		return emitReport(cmd, "configure", false, data, human, 1)
	}
	return emitFailure(cmd, "configure", err)
}

// configureLockedLine renders ReviewGrainLockedError's three cases (a
// recorded PR, a recorded review ref, or a terminal status) as the plain
// human sentence, referencing the node by the shared "key (id)" convention
// rather than the error's own bare ident().
func configureLockedLine(tree *store.Tree, e *mutate.ReviewGrainLockedError) string {
	ref := gen.RefByID(tree, e.ID)
	switch {
	case e.PR != nil:
		return fmt.Sprintf("%s %s already has a review surface recorded (pr #%d); review grain cannot change once its own review surface exists.",
			e.Noun, ref, *e.PR)
	case e.ReviewRef != nil:
		return fmt.Sprintf("%s %s already has a review surface recorded (review_ref branch %s); review grain cannot change once its own review surface exists.",
			e.Noun, ref, e.ReviewRef.Branch)
	default:
		return fmt.Sprintf("%s %s is terminal (%s); review grain cannot change once its own review surface exists.",
			e.Noun, ref, e.Status)
	}
}
