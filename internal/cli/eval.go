package cli

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/gates"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/spf13/cobra"
)

// This file implements `tiller eval <noun> <node>` (cli-surface.md, "Gates, evals,
// and attestation"): the narrowest acceptance tool, the one the agent runs while
// working out whether acceptance is met and exactly what the required `evals` gate
// shells to. It is a thin shell over internal/core/mutate.Eval: it validates the
// noun against the PR grains that carry evals (feature, task), resolves the address,
// and renders one of three outcomes.
//
// Three modes share the command. The default run evaluates the mechanical criteria
// and records the evals gate; a required criterion that failed is a BLOCKED business
// answer (exit 1) carrying the per-criterion data, never an error block. With
// --emit-rubrics it prints the rubric packet for the agent to grade (a SUCCESS whose
// data is the packet). With --criterion <id> --record-rubric-output <file> it
// validates the agent's graded output and moves a required criterion to
// needs_attestation. A bad address, an unreadable or malformed output, or a missing
// owner/git is a tool failure (exit 2/3) with an error block.

// EvalView is the data block of a default or record eval run: the node, its
// unchanged lifecycle status, whether any required criterion failed, the criterion
// just recorded (record mode), and the per-criterion results in eval-file order.
type EvalView struct {
	Noun     string          `json:"noun"`
	ID       string          `json:"id"`
	Key      string          `json:"key,omitempty"`
	Status   string          `json:"status"`
	Passed   bool            `json:"passed"`
	Recorded string          `json:"recorded,omitempty"`
	Criteria []CriterionView `json:"criteria"`
	Changed  bool            `json:"changed"`
}

// CriterionView is one criterion in the eval envelope: its id, binding kind, the
// resulting per-criterion status, whether it is required, and, for a default
// run, whether a recorded pass stood in for a fresh run (design
// 2026-10-21-gate-reuse).
type CriterionView struct {
	ID       string `json:"id"`
	Binding  string `json:"binding,omitempty"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Reused   bool   `json:"reused,omitempty"`
}

func newEvalCmd() *cobra.Command {
	var emit, rerun bool
	var criterion, record string
	cmd := &cobra.Command{
		Use:   "eval <noun> <node>",
		Short: "Run a node's acceptance criteria and hand off the rubric evals.",
		Long: "eval runs a node's acceptance contract: it runs the test and command\n" +
			"bindings, reports manual bindings as pending-human, and emits a rubric\n" +
			"packet for eval bindings. --emit-rubrics writes the packet JSON for the\n" +
			"agent to grade; --criterion <id> --record-rubric-output <file> validates\n" +
			"the graded output and moves a required criterion to needs_attestation.\n" +
			"A required eval criterion is satisfied only by a human `tiller attest`.\n" +
			"A command or test criterion that already passed on unchanged code under an\n" +
			"unchanged command is reused, not rerun; --rerun runs every one for real.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEval(cmd, args[0], args[1], emit, rerun, criterion, record)
		},
	}
	cmd.Flags().BoolVar(&rerun, "rerun", false, "run every command and test criterion for real, even one whose recorded pass could be reused")
	cmd.Flags().BoolVar(&emit, "emit-rubrics", false, "write the rubric packet JSON for the agent to grade")
	cmd.Flags().StringVar(&criterion, "criterion", "", "the eval criterion id to record rubric output for")
	cmd.Flags().StringVar(&record, "record-rubric-output", "", "path to a graded rubric output file to record as evidence")
	addContributorFlag(cmd)
	return cmd
}

// runEval validates the flag combination, resolves the node, runs the requested
// eval mode through the core, and renders the packet, the success, or the blocked
// report. --rerun only means something to a default run, so pairing it with
// either rubric mode is the same mode conflict.
func runEval(cmd *cobra.Command, noun, node string, emit, rerun bool, criterion, record string) error {
	if rerun && (emit || record != "") {
		return emitFailure(cmd, "eval", &usageError{
			reason: "eval_mode_conflict",
			msg:    "--rerun applies to a default run; it cannot be combined with --emit-rubrics or --record-rubric-output",
		})
	}
	if emit && record != "" {
		return emitFailure(cmd, "eval", &usageError{
			reason: "eval_mode_conflict",
			msg:    "--emit-rubrics and --record-rubric-output cannot be combined",
		})
	}
	if (record != "") != (criterion != "") {
		return emitFailure(cmd, "eval", &usageError{
			reason: "record_needs_criterion",
			msg:    "--record-rubric-output and --criterion must be given together",
		})
	}

	tree, n, err := resolveForMutation(cmd, "eval", noun, node, []string{store.NounFeature, store.NounTask})
	if err != nil {
		return err
	}

	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "eval", err)
	}
	rev := vcs.New(tree.Root)
	res, err := mutate.Eval(tree, d, rev, n, mutate.EvalInput{
		EmitRubrics: emit, Criterion: criterion, RecordRubricOutput: record, Progress: newProgress(cmd),
		Rerun: rerun,
	})
	if err != nil {
		return emitFailure(cmd, "eval", err)
	}

	ref := gen.Ref(tree, n)
	if emit {
		return emitSuccess(cmd, "eval", res.Packet, humanPacket(res, ref))
	}

	view := evalView(res)
	// Record mode reports a successful mutation even when the recorded rubric
	// failed. Criterion and gate status describe acceptance separately; only
	// the default run exits 1 for unsatisfied required criteria.
	if res.Recorded != "" {
		return emitSuccess(cmd, "eval", view, humanEval(res, ref))
	}
	if res.Passed {
		return emitSuccess(cmd, "eval", view, humanEval(res, ref))
	}
	return emitReport(cmd, "eval", false, view, humanEval(res, ref), 1)
}

// evalView projects the core result onto the JSON data block. A criterion's
// reused flag is derived from a stored timestamp, so in the record mode it
// could report a reuse from an earlier run; it is emitted only for the default
// run.
func evalView(res mutate.EvalResult) EvalView {
	crits := make([]CriterionView, len(res.Criteria))
	for i, c := range res.Criteria {
		crits[i] = CriterionView{
			ID: c.ID, Binding: c.Binding, Status: c.Status, Required: c.Required,
			Reused: c.Reused && res.Recorded == "",
		}
	}
	return EvalView{
		Noun: res.Noun, ID: res.ID, Key: res.Key, Status: res.Status,
		Passed: res.Passed, Recorded: res.Recorded, Criteria: crits, Changed: res.Changed,
	}
}

// humanPacket is the plain-text handoff for an emit run. It does not just count the
// packets: for each eval-binding criterion it surfaces the output_path the agent must
// write its graded RubricOutput to and the rubric guidance, so an agent grading
// without --json still knows exactly where each packet goes and what to grade (B4).
func humanPacket(res mutate.EvalResult, ref string) string {
	var crits []gates.RubricCriterion
	if res.Packet != nil {
		crits = res.Packet.Criteria
	}
	if len(crits) == 0 {
		return fmt.Sprintf("No eval-binding criteria to grade on %s %s.", res.Noun, ref)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Emitted %d rubric packet(s) for %s %s. Grade each, write the\n", len(crits), res.Noun, ref)
	b.WriteString("graded rubric output (schema tiller.rubric_output.v1) to its output_path,\n")
	b.WriteString("then record it with `tiller eval ... --criterion <id> --record-rubric-output <file>`:\n")
	for _, c := range crits {
		fmt.Fprintf(&b, "\n- %s: %s\n", c.ID, c.Statement)
		fmt.Fprintf(&b, "  output_path: %s\n", c.OutputPath)
		if c.Rubric != "" {
			fmt.Fprintf(&b, "  rubric: %s", c.Rubric)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// humanEval is the plain-text confirmation for a default or record run. The
// node is named by the caller-computed reference ("<key-path> (<id>)"). A
// default run follows its first line with the criteria that stood a recorded
// pass in for a run; the record mode never does (see evalView).
func humanEval(res mutate.EvalResult, ref string) string {
	if res.Recorded != "" {
		for _, c := range res.Criteria {
			if c.ID != res.Recorded {
				continue
			}
			if c.Status == store.CriterionStatusNeedsAttestation {
				return fmt.Sprintf("Recorded rubric output for %s on %s %s: now needs human attestation.", res.Recorded, res.Noun, ref)
			}
			return fmt.Sprintf("Recorded rubric output for %s on %s %s: criterion status is %s.", res.Recorded, res.Noun, ref, c.Status)
		}
		return fmt.Sprintf("Recorded rubric output for %s on %s %s.", res.Recorded, res.Noun, ref)
	}
	var reused []string
	for _, c := range res.Criteria {
		if c.Reused {
			reused = append(reused, c.ID)
		}
	}
	note := ""
	if len(reused) > 0 {
		note = "\nReused: " + strings.Join(reused, ", ") + "."
	}
	if res.Passed {
		return fmt.Sprintf("Evaluated %s %s: every required criterion is satisfied.", res.Noun, ref) + note
	}
	// A blocked run names every required criterion that is not yet a pass, with its
	// status, so the agent sees both a red (fail) and a not-yet (pending,
	// needs_attestation, stale) criterion rather than only failures.
	var unmet []string
	for _, c := range res.Criteria {
		if c.Required && c.Status != store.CriterionStatusPass {
			unmet = append(unmet, fmt.Sprintf("%s (%s)", c.ID, c.Status))
		}
	}
	return fmt.Sprintf("Blocked: %s %s has unsatisfied required criterion(s): %s.", res.Noun, ref, strings.Join(unmet, ", ")) + note
}
