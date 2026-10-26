package views

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// decomposeReason says why decompose is due on a tree with no epics: its
// to-be design docs await decomposition (docs/spec/cli-surface.md). Callers
// check that a design doc exists.
func decomposeReason(t *store.Tree) string {
	n := len(t.ToBeDesign)
	return fmt.Sprintf("no epics recorded yet; %d to-be design %s %s decomposition, starting with .tiller/design/%s",
		n, plural(n, "doc", "docs"), plural(n, "awaits", "await"), t.ToBeDesign[0])
}

// unplannedReason is the end state's human-owned Next. It names the open
// question count only when nonzero, and the ways work enters for the
// project's kind.
func unplannedReason(t *store.Tree) string {
	open := 0
	for _, q := range t.Questions {
		if gen.QuestionOpen(q) {
			open++
		}
	}
	questions := ""
	if open > 0 {
		questions = fmt.Sprintf("%d open %s", open, plural(open, "question", "questions"))
	}
	if t.Manifest != nil && (t.Manifest.Project.Kind == store.KindBrownfield || t.Manifest.Project.Kind == store.KindLegacyMigration) {
		facts := "baseline established"
		if questions != "" {
			facts += ", " + questions
		}
		return "no work is planned yet (" + facts + "): bring work in through intake (a document making a case for change), capture (a bug, chore, or tweak), or a to-be design under .tiller/design/ (which makes decompose due)"
	}
	lead := "no work is planned yet"
	if questions != "" {
		lead += " (" + questions + ")"
	}
	return lead + ": add design inputs under .tiller/design/ for decompose to turn into a draft tree"
}
