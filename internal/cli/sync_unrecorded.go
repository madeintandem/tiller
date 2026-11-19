package cli

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
)

// UnrecordedMergeView is one bug, chore, or tweak that commits on the
// integration branch name while the node is still ready, in_progress, or
// check. Sync recorded nothing for it: no crossing, no status change, no event.
//
// commits lists every naming commit in the scanned window, newest first, by
// full object id. next is the command the warning hands the reader, and warning
// is the same sentence the human output prints, so a caller that only relays
// text needs nothing else.
type UnrecordedMergeView struct {
	Noun    string             `json:"noun"`
	ID      string             `json:"id"`
	Key     string             `json:"key,omitempty"`
	Status  string             `json:"status"`
	Commits []NamingCommitView `json:"commits"`
	Next    string             `json:"next"`
	Warning string             `json:"warning"`
}

// NamingCommitView is one commit whose message names the node.
type NamingCommitView struct {
	Commit  string `json:"commit"`
	Subject string `json:"subject"`
}

func unrecordedMergeViews(merges []mutate.UnrecordedMerge) []UnrecordedMergeView {
	var views []UnrecordedMergeView
	for _, m := range merges {
		commits := make([]NamingCommitView, len(m.Commits))
		for i, c := range m.Commits {
			commits[i] = NamingCommitView{Commit: c.Commit, Subject: c.Subject}
		}
		views = append(views, UnrecordedMergeView{
			Noun: m.Noun, ID: m.ID, Key: m.Key, Status: m.Status,
			Commits: commits, Next: m.NextStep(), Warning: m.Warning(),
		})
	}
	return views
}

// humanSyncUnrecordedMerges renders the warning block: one line per node, each
// naming the node, its status, the commit, and the next step, then what the
// finding means. Every node is named, with no cap: a warning that summarized
// its tail would leave some merges as unreported as before. It renders on
// every outcome, since a run that crossed other
// work can still have met a merge it could not record.
func humanSyncUnrecordedMerges(res mutate.SyncResult) string {
	merges := res.UnrecordedMerges
	if len(merges) == 0 {
		return ""
	}
	lines := []string{fmt.Sprintf("Warning: merged into %s with no recorded submission (%d):", res.Into, len(merges))}
	for _, m := range merges {
		lines = append(lines, "  - "+m.Warning())
	}
	lines = append(lines,
		"  Sync recorded nothing for these and moved no status: it observes a bug, chore, or tweak only once"+
			" it is in review. Each still has to go through its own start, check, and submit before a sync"+
			" can record it done.")
	return strings.Join(lines, "\n")
}
