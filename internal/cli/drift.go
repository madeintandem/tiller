package cli

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file wires the project-level `tiller drift` command (cli-surface.md,
// the drift paragraph, sentence by sentence). Like sync.go, it is a thin
// shell: load the tree, hand a read-only git window to drift.Run, and render
// what the sweep found through views.DriftReportOf, the pure projection Task
// 11's serve endpoint reuses verbatim so the CLI and the console cannot
// disagree.
//
// drift refuses nothing and mutates nothing (the paragraph's own words), so
// its exit code is not a refusal signal: exit 0 asserts coverage, every
// in-scope node was compared and nothing moved, and exit 1 means the sweep
// found candidates, uncovered nodes, or both, because a run that could not
// check a node must never look like a run that found nothing. That is why
// the blocked path here carries the identical data shape the clean path
// does (DriftView, below) rather than a dedicated refusal block the way an
// illegal transition or a structural blocker does elsewhere in this
// package: nothing was refused, the read completed in full, and what it
// found is exactly the report.
//
// The one error drift.Run ever returns, an unknown drift.checks name, already
// implements store.ExitCoder and envelope.go's reasoner interface (task-10-
// brief.md's engine-contracts note), so emitFailure's existing classify
// switch handles it without a new arm.

// reasonDriftFound is the BLOCKED data's own reason token. It is set only
// when the sweep found something; the clean run leaves it empty, which
// omitempty then drops, so the two outcomes really do share one shape.
const reasonDriftFound = "drift_found"

// markerOrder is the schema's own fixed marker order (artifact-schema.md,
// "Drift control hooks"; the same order drift.Run's own sweep and the
// manifest's example drift.checks list use), exported by the engine as
// drift.KnownMarkers so this file's
// human and footer rendering walks the identical order the engine validates
// drift.checks against, rather than a second, driftable copy of it.
var markerOrder = drift.KnownMarkers

// DriftView is the JSON data block `tiller drift` emits, clean or found
// alike: the pure report projection plus Reason, populated only on the
// blocked path. There is deliberately no
// second, differently-shaped type for findings: exit 1 here means the sweep
// completed and is reporting what it found, not that the command was
// refused, so a caller already parsing the clean shape needs nothing new to
// read the blocked one.
type DriftView struct {
	views.DriftReportView
	Reason string `json:"reason,omitempty"`
}

func newDriftCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drift",
		Short: "Sweep the enabled drift checks and report what moved.",
		Long: "drift is mechanical detection over the markers the schema plants:\n" +
			"stale_spec, active_basis, answered_question, missing_surface,\n" +
			"orphan_eval, stale_design, and shifted_design. It runs every enabled check,\n" +
			"reporting candidates (what moved, and since when, keyed by marker) and\n" +
			"uncovered nodes (what a check could not compare, and why) as two\n" +
			"separate lists, plus a footer naming every disabled check and every\n" +
			"node a recorded human decision exempted from a marker's scope.\n" +
			"\n" +
			"Exit 0 asserts coverage: every in-scope node was compared and nothing\n" +
			"moved. Exit 1 means the sweep found candidates, uncovered nodes, or\n" +
			"both, because a run that could not check a node must never look like a\n" +
			"run that found nothing. drift refuses nothing and mutates nothing; the\n" +
			"fix is the agent's to draft through the mend-drift skill and yours to\n" +
			"approve.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDrift(cmd)
		},
	}
}

// runDrift loads the tree, sweeps it through drift.Run, and renders the
// report. found (candidates or uncovered non-empty) is the one thing that
// decides ok/exit code; everything else about the data block is identical
// either way.
func runDrift(cmd *cobra.Command) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "drift", err)
	}

	rev := vcs.NewHeadSnapshot(vcs.New(tree.Root))
	rep, err := drift.Run(tree, rev)
	if err != nil {
		return emitFailure(cmd, "drift", err)
	}

	view := views.DriftReportOf(tree, rep)
	found := len(rep.Candidates) > 0 || len(rep.Uncovered) > 0
	data := DriftView{DriftReportView: view}
	human := humanDrift(view, found)
	if found {
		data.Reason = reasonDriftFound
		return emitReport(cmd, "drift", false, data, human, 1)
	}
	return emitReport(cmd, "drift", true, data, human, 0)
}

// humanDrift is the plain-text render for a terminal reader: one section per
// marker that carries candidates, then an Uncovered section, then the
// footer (cli-surface.md's drift paragraph).
// Clean output states what was compared rather than asserting a bare
// "no drift", because exit 0 is itself an assertion of coverage.
func humanDrift(v views.DriftReportView, found bool) string {
	var b strings.Builder
	for _, marker := range markerOrder {
		rows := v.Candidates[marker]
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s (%d):\n", marker, len(rows))
		for _, c := range rows {
			writeCandidateRow(&b, c)
		}
		b.WriteString("\n")
	}

	if len(v.Uncovered) > 0 {
		fmt.Fprintf(&b, "Uncovered (%d):\n", len(v.Uncovered))
		for _, u := range v.Uncovered {
			writeUncoveredRow(&b, u)
		}
		b.WriteString("\n")
	}

	writeDriftFooter(&b, v, found)
	return strings.TrimRight(b.String(), "\n")
}

// writeCandidateRow renders one candidate in the reference convention
// (<key-path> (<id>)), with its files, its since anchor as a short sha (the
// human renderer's own business; the JSON payload carries the anchor
// structurally and unabridged), and its context lines, each indented beneath
// the row.
func writeCandidateRow(b *strings.Builder, c views.CandidateView) {
	fmt.Fprintf(b, "  - %s %s: %s\n", c.Noun, c.Ref, c.Detail)
	if len(c.Files) > 0 {
		fmt.Fprintf(b, "      files: %s\n", strings.Join(c.Files, ", "))
	}
	if sha := shortSHA(c.Since); sha != "" {
		fmt.Fprintf(b, "      since: %s\n", sha)
	}
	if c.Until != nil {
		fmt.Fprintf(b, "      range: %s..%s\n", shortSHA(c.Since), shortSHA(c.Until))
	}
	for _, feature := range c.Features {
		fmt.Fprintf(b, "      feature: %s (%s); spec: %s; crossed: %s\n", feature.Key, feature.ID, feature.Spec, shortSHA(&feature.Commit))
	}
	if c.Noun == "design" && c.Remedy != "" {
		fmt.Fprintf(b, "      after human review: %s\n", c.Remedy)
	}
	for _, ctx := range c.Context {
		fmt.Fprintf(b, "      context: %s\n", ctx)
	}
}

// writeUncoveredRow renders one uncovered node: its marker (the Uncovered
// list is flat across every marker, so the row has to carry it, unlike a
// candidate row which already sits inside its marker's own section), its
// reference, and its reason plus detail.
func writeUncoveredRow(b *strings.Builder, u views.UncoveredView) {
	fmt.Fprintf(b, "  - [%s] %s %s: %s", u.Marker, u.Noun, u.Ref, u.Reason)
	if u.Detail != "" {
		fmt.Fprintf(b, " (%s)", u.Detail)
	}
	b.WriteString("\n")
}

// writeDriftFooter renders the footer every run carries, clean or found
// alike: the compared count (the sentence that asserts coverage on a clean
// run, cli-surface.md's own words, rather than a bare "no drift"), the
// disabled checks by name, the exemptions by marker, and the coverage per
// marker with its own unit named: work nodes, design docs, or criteria for
// orphan_eval. Summing across markers as if those units matched
// would misstate both. Counted nouns follow the house "(s)" convention
// (sync.go's "done crossing(s)", "node(s) still in review"; show.go's
// "surface(s)"), the same one this file's earlier draft departed from.
func writeDriftFooter(b *strings.Builder, v views.DriftReportView, found bool) {
	checksRun := len(markerOrder) - len(v.Disabled)
	compared := fmt.Sprintf("%d node(s)", v.Compared)
	if v.ComparedDesign > 0 || len(v.Coverage[drift.MarkerStaleDesign]) > 0 || len(v.Coverage[drift.MarkerShiftedDesign]) > 0 {
		compared += fmt.Sprintf(" and %d doc(s)", v.ComparedDesign)
	}
	if found {
		fmt.Fprintf(b, "Compared %s across %d check(s).\n", compared, checksRun)
	} else {
		fmt.Fprintf(b, "Compared %s across %d check(s); nothing moved.\n", compared, checksRun)
	}

	if len(v.Disabled) > 0 {
		fmt.Fprintf(b, "Disabled: %s\n", strings.Join(v.Disabled, ", "))
	}

	if len(v.Exempted) > 0 {
		var parts []string
		for _, m := range markerOrder {
			if n := v.Exempted[m]; n > 0 {
				parts = append(parts, fmt.Sprintf("%s: %d", m, n))
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(b, "Exempted: %s\n", strings.Join(parts, ", "))
		}
	}

	if len(v.Coverage) > 0 {
		b.WriteString("Coverage:\n")
		for _, m := range markerOrder {
			if buckets, ok := v.Coverage[m]; ok {
				fmt.Fprintf(b, "  %s: %s\n", m, coverageLine(m, buckets))
			}
		}
	}
}

// coverageBucketOrder is the fixed display order of the four bucket names
// every marker's Coverage map uses (drift.go's own bumpCoverage call sites),
// paired with the human label each renders as. "out_of_scope" and
// "not_yet_in_scope" read with spaces for a terminal reader; the raw token
// rides unchanged in the JSON payload, since Coverage there is drift.Report's
// own map, untouched by this rendering.
var coverageBucketOrder = []struct{ key, label string }{
	{"checked", "checked"},
	{"unresolvable", "unresolvable"},
	{"out_of_scope", "out of scope"},
	{"not_yet_in_scope", "not yet in scope"},
}

// coverageLine renders one marker's bucket counts with its unit named once,
// on the first clause, matching the drift paragraph's own example:
// "orphan_eval: 3 criteria checked, 1 unresolvable, 2 out of scope". A bucket
// absent or zero is dropped rather than printed as a zero clause.
func coverageLine(marker string, buckets map[string]int) string {
	unit := coverageUnit(marker)
	var parts []string
	for _, o := range coverageBucketOrder {
		n := buckets[o.key]
		if n == 0 {
			continue
		}
		if len(parts) == 0 {
			parts = append(parts, fmt.Sprintf("%d %s %s", n, unit, o.label))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, o.label))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("0 %s checked", unit)
	}
	return strings.Join(parts, ", ")
}

// coverageUnit names the unit one marker's Coverage bucket counts in:
// criteria for orphan_eval, doc(s) for baseline markers, and node(s) for
// work-node markers (drift.Report's Coverage contract).
// "criteria" is left unpluralized: it is already the irregular plural of
// "criterion", and the house "(s)" convention applies to a
// regular noun's own suffix, which "criteria" has none of; the schema's own
// worked example ("3 criteria checked") uses it the same invariant way.
func coverageUnit(marker string) string {
	if marker == drift.MarkerStaleDesign || marker == drift.MarkerShiftedDesign {
		return "doc(s)"
	}
	if marker == drift.MarkerOrphanEval {
		return "criteria"
	}
	return "node(s)"
}

// shortSHA renders a projected revision anchor as a short sha for a terminal
// reader, the human renderer's own business; the JSON payload carries r's value
// verbatim through views.RevisionRefView, untouched by this. A nil anchor (no
// commit to date the candidate by) and a non-git kind (sha256) both pass
// through as "" or unchanged respectively, since only a git value is a sha to
// shorten at all.
func shortSHA(r *views.RevisionRefView) string {
	if r == nil {
		return ""
	}
	const prefix = "git:"
	if !strings.HasPrefix(r.Value, prefix) {
		return r.Value
	}
	hex := strings.TrimPrefix(r.Value, prefix)
	if len(hex) > 7 {
		hex = hex[:7]
	}
	return prefix + hex
}
