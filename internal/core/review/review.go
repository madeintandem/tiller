// Package review reports unpublished task-review state from read-only Git
// observations shared by the CLI and console.
package review

import (
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// Facts names the unpublished task-branch tip and the upstream compared against.
type Facts struct {
	Commit   string
	Branch   string
	Upstream string
}

// ForTask answers whether taskID's review state has outrun what its PR can
// show a reviewer: an in-review task that records a PR, whose own branch
// holds a commit its remote-tracking counterpart does not.
// Only the PR path is ever asked anything: a remote-less submit's branch-ref
// fallback (offlineReviewRef, internal/cli) leaves PR nil, so there is
// nothing here to compare and this returns nil before touching git at all,
// the mechanical form of the global constraint that the offline path never
// warns.
//
// taskID need not already be known to name a task: a caller that holds any
// node's id (the node page, in particular, which addresses every noun
// through one handler) can pass it directly rather than type-asserting
// first, because tree.Tasks simply has no entry for an epic, feature, or
// question id and this returns nil before any git read the same way a task
// that is not in_review does.
//
// A git read that cannot answer, for any reason (no remote counterpart at
// all, a branch that was never actually pushed or checked out, or a genuine
// git fault) degrades to nil rather than propagating an error: every caller
// of this function is a report, not a gate, and UnpushedTip's own ok=false is
// already the deliberate "nothing to compare" signal for the offline case
// (vcs/git.go). g is accepted as a parameter rather than constructed here so
// a caller that already built one (as every call site does, from its own
// tree.Root) never pays for a second.
func ForTask(tree *store.Tree, taskID string, g *vcs.Git) *Facts {
	tk, ok := tree.Tasks[taskID]
	if !ok || tk.Status != "in_review" || tk.PR == nil || tk.Branch == "" {
		return nil
	}

	unpushed, upstream, ok, err := g.UnpushedTip(tk.Branch)
	if err != nil || !ok || !unpushed {
		return nil
	}

	// tk.Branch is qualified through vcs.BranchRef before naming it to
	// ShortSHA (final review finding 1, batch 42-45): ShortSHA shells
	// rev-parse, which resolves a bare name from refs/tags before refs/heads,
	// so an unqualified read here could name a same-named tag's commit in the
	// warning instead of the branch's own. UnpushedTip's own merge-base arm
	// carries the identical qualification internally already (vcs/git.go).
	// This is the one hazard a second, uncentralized copy of this
	// composition would silently risk losing: it is invisible to a test that
	// never plants a shadowing tag, which is exactly how it slipped in
	// undetected before commit f351377 caught it.
	commit, err := g.ShortSHA(vcs.BranchRef(tk.Branch))
	if err != nil {
		return nil
	}
	return &Facts{Commit: commit, Branch: tk.Branch, Upstream: upstream}
}
