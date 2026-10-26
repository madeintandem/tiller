package cli

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/views"
)

// renderReadout turns a derived readout into the markdown the agent
// reproduces verbatim into its chat reply and the terminal prints when a
// developer runs the command themselves; both audiences read the same text.
// The vocabulary is deliberately small: bold labels,
// backticks, and dash bullets. No headings, because the block sits inside the
// agent's own message and would fight its structure; no tables, because they
// render inconsistently across harnesses
// (docs/spec/cli-surface.md).
//
// renderReadout lives in package cli, not internal/gen, because
// internal/views already imports internal/gen: a gen renderer consuming a
// views.Readout would close an import cycle. gen renders committed artifacts
// (orientation.md); cli renders command output, and the readout is
// command output. renderTreeHuman (tree.go) is the precedent this follows,
// down to the doc-comment shape.
func renderReadout(r views.Readout) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s**", r.Headline)
	if r.Artifact != "" {
		fmt.Fprintf(&b, " · committed at `%s`", r.Artifact)
	}
	b.WriteString("\n")

	if len(r.Entities) > 0 {
		b.WriteString("\n**Produced**\n\n")
		for _, e := range r.Entities {
			fmt.Fprintf(&b, "- %s", e.Ref)
			if e.Note != "" {
				fmt.Fprintf(&b, " · %s", e.Note)
			}
			b.WriteString("\n")
			// The review-state warning is its own dash-bulleted
			// row directly under the node it is about, never folded onto the
			// Ref/Note line above: the wording is a full sentence, the same
			// one submit and status render verbatim (unpushedReviewWarning,
			// unpushed_review.go), and this is the one place that shape earns
			// the leading dash every other Produced row already carries.
			if e.Unpushed != nil {
				fmt.Fprintf(&b, "- %s\n", unpushedReviewWarning(e.Unpushed.Commit, e.Unpushed.Branch))
			}
		}
		if r.Withheld > 0 {
			fmt.Fprintf(&b, "- %d more\n", r.Withheld)
		}
	}

	renderBaseline(&b, r.Baseline)

	if len(r.Checks) > 0 {
		b.WriteString("\n**Verification**\n\n")
		for _, c := range r.Checks {
			// c.Result is an ordinary result string, including the literal
			// "not evaluated" a check renders when its caller supplied no
			// data (task-3-report.md, the nil-versus-empty discipline): it
			// prints exactly like any other result, no quoting or
			// special-casing, so the renderer never mangles the distinction
			// the check layer worked to preserve.
			fmt.Fprintf(&b, "- %s: %s\n", c.Label, c.Result)
		}
	}

	// Nonempty attention is one contiguous count list. Concrete queue entries
	// and their mutation commands stay behind each inline, read-only queue
	// command; an empty queue keeps the established standalone sentence.
	if len(r.NeedsYou) == 0 {
		if r.Baseline == nil || (r.Baseline.Review == nil && r.Baseline.Repair == nil) {
			b.WriteString("\nNothing needs you right now.\n")
		}
	} else {
		b.WriteString("\n**Needs attention**\n\n")
		for _, a := range r.NeedsYou {
			fmt.Fprintf(&b, "- %s", a.Text)
			if a.Command != "" {
				fmt.Fprintf(&b, " · `%s`", a.Command)
			}
			b.WriteString("\n")
		}
	}

	if r.Queue != "" {
		fmt.Fprintf(&b, "\n%s\n", r.Queue)
	}

	b.WriteString(renderReadoutNext(r.Next))
	if len(r.GrainNotes) > 0 {
		b.WriteString("\n")
		for _, note := range r.GrainNotes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
	}
	return b.String()
}

// renderCorrection renders the correction block, the lighter
// sibling of renderReadout for `tiller readout <seam> --since <sha>`. Same
// vocabulary, same two audiences, same wire format: the headline names the
// commit under the full block's own rule and then the since sha, Changed
// lists what the event stream recorded (or says plainly that it recorded
// nothing), and the queue line and the ranked Next list close the block
// exactly as the full one renders them. Work Produced, Verification, and
// Needs attention are absent; baseline coverage remains complete so a
// correction still offers the whole document review surface rather than
// hiding unchanged mappings.
func renderCorrection(c views.Correction) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s**", c.Headline)
	if c.Artifact != "" {
		fmt.Fprintf(&b, " · committed at `%s`", c.Artifact)
	}
	fmt.Fprintf(&b, " (since `%s`)\n", c.Since)

	b.WriteString("\n**Changed**\n\n")
	if len(c.Changed) == 0 {
		fmt.Fprintf(&b, "- nothing recorded since `%s`\n", c.Since)
	}
	for _, line := range c.Changed {
		fmt.Fprintf(&b, "- %s\n", line)
	}

	renderBaseline(&b, c.Baseline)

	if c.Queue != "" {
		fmt.Fprintf(&b, "\n%s\n", c.Queue)
	}

	b.WriteString(renderReadoutNext(c.Next))
	return b.String()
}

func renderBaseline(b *strings.Builder, baseline *views.BaselineReadout) {
	if baseline == nil {
		return
	}
	if baseline.Repair != nil {
		b.WriteString("\n**Baseline repair**\n\n")
		fmt.Fprintf(b, "- %s\n", baseline.Repair.Text)
	}
	if baseline.Docs == nil {
		if baseline.Review != nil {
			b.WriteString("\n**Baseline review**\n\n")
			fmt.Fprintf(b, "- %d design docs await human confirmation; inspect them with `tiller readout derive-baseline`.\n", baseline.Pending)
		}
		return
	}
	b.WriteString("\n**Recorded baseline**\n\n")
	fmt.Fprintf(b, "- Ledger docs: %d · confirmed: %d · unconfirmed: %d\n", len(baseline.Docs), baseline.Confirmed, baseline.Pending)
	fmt.Fprintf(b, "- Recorded source revisions: %d\n", len(baseline.Sources))
	fmt.Fprintf(b, "- Open questions: %d\n", baseline.OpenQuestions)
	for _, doc := range baseline.Docs {
		fmt.Fprintf(b, "- `.tiller/design/%s` · describes: %s", doc.Path, strings.Join(doc.Describes, ", "))
		if doc.Present != nil && !*doc.Present {
			b.WriteString(" · MISSING PROSE: restore the doc or remove its links with `tiller unlink design`")
		}
		if doc.Confirmed == nil {
			b.WriteString(" · unconfirmed\n")
		} else {
			fmt.Fprintf(b, " · confirmed by %s at %s (observed %s)\n", doc.Confirmed.By, doc.Confirmed.Commit.Value, doc.Confirmed.Commit.ObservedAt)
			for _, surface := range doc.Confirmed.Surfaces {
				fmt.Fprintf(b, "  - `%s` · %s %s · observed %s\n", surface.Path, surface.Kind, surface.Value, surface.ObservedAt)
				fmt.Fprintf(b, "    - Recorded inventory: %s\n", strings.Join(surface.Inventory, ", "))
			}
		}
	}
	if len(baseline.Sources) > 0 {
		b.WriteString("\n**Recorded source-to-doc links**\n\n")
		for _, source := range baseline.Sources {
			fmt.Fprintf(b, "- `%s` · %s %s → %s", source.Path, source.Kind, source.Value, strings.Join(source.Docs, ", "))
			if source.Kind == "git_blob" {
				fmt.Fprintf(b, " · recover with `git show %s`", strings.TrimPrefix(source.Value, "git:"))
			}
			b.WriteString("\n")
		}
	}
	if baseline.Review != nil {
		b.WriteString("\n**Baseline review**\n\n")
		fmt.Fprintf(b, "- %d design docs await human confirmation. Review the existing docs; do not regenerate them merely because confirmation is pending.\n", baseline.Pending)
	}
}

// renderReadoutNext closes a block on the ranked list: the **Next:** label on
// its own line, then the shortlist, the count of moves left out, and any hint,
// the same lines `tiller next` prints. A caught-up checkout still gets the
// label, followed by the caught-up text, so the block always ends on what comes
// next.
func renderReadoutNext(n views.Next) string {
	return "\n**Next:**\n" + renderNextLines(n, false) + "\n"
}
