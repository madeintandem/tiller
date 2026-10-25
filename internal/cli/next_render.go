package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file renders a ranked next list as text. `tiller next` prints it and
// the readout, bare guide, and the submit and check surfaces reuse it, so every
// surface shows the same lines (ranked-next design, decision 11).

// renderNextHeading is the line above a ranked list: who it is ranked for, or
// the nudge to join when no reader resolved.
func renderNextHeading(n views.Next) string {
	if n.Reader == "" {
		return "Up next (not personalized; run `tiller join`):"
	}
	return "Up next for " + n.Reader + ":"
}

// renderNextLines is the body under the heading: the numbered moves (the
// shortlist, or every move when all is set), the count of moves left out, and
// any hint on its own line, worded as a sentence. An empty list renders the
// caught-up text instead, followed by its hint. Callers pass the uncut
// views.Next: it cuts with Shortlist itself.
func renderNextLines(n views.Next, all bool) string {
	var lines []string
	shown := n.Candidates
	if !all {
		shown = n.Shortlist()
	}
	if len(n.Candidates) == 0 {
		lines = append(lines, views.CaughtUpText)
	}
	for i, c := range shown {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, views.NextLine(c)))
	}
	switch more := len(n.Candidates) - len(shown); {
	case more == 1:
		lines = append(lines, "1 more legal move; `tiller next --all` lists it.")
	case more > 1:
		lines = append(lines, fmt.Sprintf("%d more legal moves; `tiller next --all` lists them.", more))
	}
	if n.Hint != "" {
		lines = append(lines, views.Sentence(n.Hint))
	}
	return strings.Join(lines, "\n")
}

// renderNextText is the whole text: the heading above a list, or the caught-up
// text alone when nothing is legal.
func renderNextText(n views.Next, all bool) string {
	if len(n.Candidates) == 0 {
		return renderNextLines(n, all)
	}
	return renderNextHeading(n) + "\n" + renderNextLines(n, all)
}

// appendUpNext is the completing commands' short block (decision 9): human plus
// a blank line, "**Up next:**", and the ranked lines for the acting
// contributor, with no heading. It loads the tree afresh so the list reflects
// the mutation that just landed. The returned Next is the uncut one; a nil Next
// means the list could not be computed, and human comes back unchanged: a
// reporting problem never turns a finished act into a failure.
func appendUpNext(cmd *cobra.Command, root string, human string) (string, *views.Next) {
	tree, err := store.Load(root)
	if err != nil {
		return human, nil
	}
	next := views.NextOf(tree, readinput.ForNext(tree, resolveActingContributor(cmd, root), time.Now()))
	return human + "\n\n**Up next:**\n" + renderNextLines(next, false), &next
}

// withUpNext appends the Up next block to a success report: the lines to
// human, and the shortlist as a "next" field beside data's own, the same cut
// `tiller next --json` carries. When the list cannot be computed, both come
// back unchanged.
func withUpNext(cmd *cobra.Command, root string, data any, human string) (any, string) {
	human, next := appendUpNext(cmd, root, human)
	if next == nil {
		return data, human
	}
	cut := *next
	cut.Candidates = next.Shortlist()
	return withNext(data, cut), human
}
