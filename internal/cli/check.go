package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/spf13/cobra"
)

// This file implements the node-grain `tiller check <noun> <node>` verb
// (cli-surface.md, "Gates, evals, and attestation"). It is a thin shell: it
// validates the noun against the PR grains that carry gates (feature, task,
// bug, chore, tweak), resolves the address, hands a clean request to
// internal/core/mutate.Check, and
// renders one of three envelopes. The reasoning, gate execution, gates-block
// recording, state advance, and event live in the core; the `against` revision
// comes from internal/vcs (read-only git).
//
// The three outcomes mirror the lifecycle verbs. All required gates green is a
// SUCCESS (exit 0): the node's recorded gates, and for a task its advance to the
// check state. An unsatisfied required gate is a BLOCKED business answer (exit 1): an
// ok:false report carrying the gate statuses in a data block, never an error
// block, so the agent sees exactly which gate to fix. A bad address, an unknown
// owner, or a missing git is a tool failure (exit 2/3) with an error block.

// CheckView is the data block of a check: the node it ran against, the resulting
// state, whether every required gate is green, and the per-gate results. It is the
// payload of both the success envelope and the blocked report, so the agent reads
// the same shape whether check passed or a gate is red.
type CheckView struct {
	Noun    string           `json:"noun"`
	ID      string           `json:"id"`
	Key     string           `json:"key,omitempty"`
	Status  string           `json:"status"`
	Passed  bool             `json:"passed"`
	Changed bool             `json:"changed"`
	Gates   []GateResultView `json:"gates"`
	// Warnings are advisory lines the check still succeeded under.
	Warnings []string `json:"warnings,omitempty"`
}

// GateResultView is one recorded gate in the check envelope: its name, whether it
// is required, its kind, the resulting status, the revision it ran against, and
// the evidence path. The run timestamp and owner live in the recorded block on
// disk; the envelope carries what the agent branches on. Reused marks a recorded
// pass that stood in for a fresh run (ReusedFrom names the source node when it
// is not this one); NotRun marks a gate a `--gate` run was not allowed to run
// (design 2026-10-21-gate-reuse).
type GateResultView struct {
	Name       string `json:"name"`
	Required   bool   `json:"required"`
	Kind       string `json:"kind,omitempty"`
	Status     string `json:"status"`
	Against    string `json:"against,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
	Reused     bool   `json:"reused,omitempty"`
	ReusedFrom string `json:"reused_from,omitempty"`
	NotRun     bool   `json:"not_run,omitempty"`
}

// runCheck validates the active-basis guard inputs, resolves the node, runs and
// records its gates through the core, and renders the success or the blocked
// report. It accepts the PR grains that carry gates (feature, task, bug, chore, tweak).
// rerun and gateNames are `--rerun` and `--gate`; a blank gate name is misuse,
// refused before anything runs.
func runCheck(cmd *cobra.Command, noun, node, assume, reason string, rerun bool, gateNames []string) error {
	if strings.TrimSpace(assume) != "" && strings.TrimSpace(reason) == "" {
		return emitFailure(cmd, "check", &usageError{
			reason: "reason_required",
			msg:    "--assume-question requires a paired --reason",
		})
	}

	var only []string
	for _, name := range gateNames {
		name = strings.TrimSpace(name)
		if name == "" {
			return emitFailure(cmd, "check", &usageError{
				reason: "blank_gate",
				msg:    "--gate needs a gate name; got a blank value",
			})
		}
		only = append(only, name)
	}

	tree, n, err := resolveForMutation(cmd, "check", noun, node, []string{store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak})
	if err != nil {
		return err
	}

	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "check", err)
	}
	rev := vcs.New(tree.Root)
	res, err := mutate.Check(tree, d, rev, n, mutate.CheckInput{
		AssumeQuestion: assume, Reason: reason, Progress: newProgress(cmd),
		Rerun: rerun, Gates: only,
	})
	if err != nil {
		return reportTransitionError(cmd, "check", tree, n, err)
	}

	view := checkView(res)
	ref := gen.Ref(tree, n)
	if res.Passed {
		data, human := withUpNext(cmd, tree.Root, view, humanCheck(res, ref))
		return emitSuccess(cmd, "check", data, human)
	}
	return emitReport(cmd, "check", false, view, humanCheck(res, ref), 1)
}

// checkView projects the core result onto the JSON data block.
func checkView(res mutate.CheckResult) CheckView {
	gates := make([]GateResultView, len(res.Gates))
	for i, g := range res.Gates {
		gates[i] = GateResultView{
			Name:     g.Name,
			Required: g.Required,
			Kind:     g.Kind,
			Status:   g.Status,
			Against:  g.Against,
			Evidence: g.EvidenceRef,

			Reused:     g.Reused,
			ReusedFrom: g.ReusedFrom,
			NotRun:     g.NotRun,
		}
	}
	return CheckView{
		Noun:     res.Noun,
		ID:       res.ID,
		Key:      res.Key,
		Status:   res.Status,
		Passed:   res.Passed,
		Changed:  res.Changed,
		Gates:    gates,
		Warnings: res.Warnings,
	}
}

// humanCheck is the plain-text confirmation for a terminal reader. The node is
// named by the caller-computed reference ("<key-path> (<id>)"). The green line
// says "state is now": a task's post-check state is itself named "check", so a
// bare "now check." read as an instruction to run the command again. A blocked
// check names each unsatisfied gate and its outstanding required criteria or
// command evidence. A criterion awaiting attestation includes the human's command.
// Both renderings follow the first line with the reuse notes: which gates stood
// a recorded pass in for a run, which a `--gate` run left alone, and each
// advisory warning.
func humanCheck(res mutate.CheckResult, ref string) string {
	if res.Passed {
		return fmt.Sprintf("Checked %s %s: all required gates green; state is now %s.", res.Noun, ref, res.Status) + reuseNotes(res)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Blocked: %s %s has unsatisfied required gate(s): ", res.Noun, ref)
	sep := ""
	for _, g := range res.Gates {
		if g.Required && g.Status != store.GateStatusPass {
			b.WriteString(sep)
			b.WriteString(g.Name)
			sep = ", "
		}
	}
	b.WriteByte('.')
	b.WriteString(reuseNotes(res))
	for _, g := range res.Gates {
		if g.Required && g.Status != store.GateStatusPass {
			b.WriteString("\n  " + unmetGateDetail(g, res.Noun, res.ID, res.AggregateBranch))
		}
	}
	return b.String()
}

// reuseNotes renders the lines that follow a check's first line, each led by a
// newline so the caller appends them directly. A reused gate names when its
// pass actually ran (UTC, minute grain) and, when a riding task's record
// carried it, which task; a gate a `--gate` run was not allowed to run is
// named as excluded.
func reuseNotes(res mutate.CheckResult) string {
	var reused, notRun []string
	for _, g := range res.Gates {
		switch {
		case g.Reused:
			when := "run " + g.ReusedRunAt.UTC().Format("2006-01-02 15:04 UTC")
			if g.ReusedFrom != "" {
				when = g.ReusedFrom + ", " + when
			}
			reused = append(reused, fmt.Sprintf("%s (%s)", g.Name, when))
		case g.NotRun:
			notRun = append(notRun, g.Name)
		}
	}
	var b strings.Builder
	if len(reused) > 0 {
		fmt.Fprintf(&b, "\nReused: %s.", strings.Join(reused, ", "))
	}
	if len(notRun) > 0 {
		fmt.Fprintf(&b, "\nNot run: %s (excluded by --gate).", strings.Join(notRun, ", "))
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(&b, "\nWarning: %s", w)
	}
	return b.String()
}

// unmetGateDetail lists outstanding required criteria in id order or names the
// command gate's status and evidence. Attestation is a human action, never an
// instruction to the agent to approve its own work. A coarse-grain feature
// names its aggregate branch, where the attest is recorded and committed.
func unmetGateDetail(g mutate.GateOutcome, noun, node, aggregate string) string {
	if len(g.Criteria) > 0 {
		var ids []string
		for id, status := range g.Criteria {
			if status != store.CriterionStatusPass {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = fmt.Sprintf("%s (%s)", id, g.Criteria[id])
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s: %s", g.Name, strings.Join(parts, ", "))
		for _, id := range ids {
			if g.Criteria[id] == store.CriterionStatusNeedsAttestation {
				fmt.Fprintf(&b, "\n  Human review required for %s; after accepting it, run: tiller attest %s %s --criterion %s --status pass",
					id, noun, node, id)
				if aggregate != "" {
					fmt.Fprintf(&b, " on branch %s", aggregate)
				}
			}
		}
		return b.String()
	}
	if g.EvidenceRef != "" {
		return fmt.Sprintf("%s: %s (evidence: %s)", g.Name, g.Status, g.EvidenceRef)
	}
	return fmt.Sprintf("%s: %s", g.Name, g.Status)
}
