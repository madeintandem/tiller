package cli

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/core/review"
	"github.com/madeintandem/tiller/internal/views"
)

// This file is the review-state warning's one shared home on the CLI side
// (issue #42): the field case where a submit-state commit sat unpushed on the
// local branch, the human merged the PR without it, and the crossing became
// unobservable because nothing said so at any of the surfaces that name a
// task's review state. unpushedReviewWarning is the one wording every
// surface renders verbatim, so an agent's prose habits can never let them
// drift.
//
// The live-git computation itself, once this file's own
// unpushedReviewForTask, now lives in internal/core/review (review.ForTask,
// moved out in the review-fix wave that followed this task): the console's
// node page (internal/serve/node.go) needs the identical read, and
// internal/serve cannot import internal/cli (internal/cli already imports
// internal/serve for the `serve` command, so the reverse import would
// cycle). Four surfaces call review.ForTask today: submit's own
// confirmation and status's oracle (lifecycle_cmds.go, status.go, both in
// this package), the execute readout (readout.go's readoutUnpushedReviews),
// and the node page.
//
// UnpushedReviewFacts is a CLI-local alias for review.Facts, the identical
// pattern status.go's own alias block already uses for views types
// (ProjectSummary, NodeStatusView, and the rest), so every existing
// signature in this package that already names UnpushedReviewFacts keeps
// reading naturally with no rename sweep.
type UnpushedReviewFacts = review.Facts

// unpushedReviewWarning is the one wording the submit confirmation, the
// execute readout, and the task status oracle all render verbatim (the
// brief's own exact text), so the surfaces it names can never each say this
// a different way. The readout's own line prepends the leading dash every
// other Produced row already uses (readout_render.go); submit and status
// render the bare sentence on its own line.
func unpushedReviewWarning(commit, branch string) string {
	return fmt.Sprintf(
		"state commit %s is not on the PR; push %s before merging or the crossing cannot be observed",
		commit, branch)
}

// unpushedReviewView projects review.Facts onto the additive unpushed_review
// JSON object (commit, branch, upstream) the status and submit envelopes
// carry, nil exactly when facts is nil so the field stays absent from an
// envelope with nothing to warn about.
func unpushedReviewView(f *UnpushedReviewFacts) *views.UnpushedReviewView {
	if f == nil {
		return nil
	}
	return &views.UnpushedReviewView{Commit: f.Commit, Branch: f.Branch, Upstream: f.Upstream}
}
