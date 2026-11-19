package mutate

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// unrecordedMergeScanLimit bounds the commits one scan reads: the most recent
// commits on the integration branch that change the project's code, which is
// anything outside Tiller's own .tiller/ and artifacts/tiller/.
//
// Nothing on disk remembers the integration commit an earlier sync observed,
// and the finding has to be derivable by a readout that never ran a sync, so
// the window is a fixed count from the tip rather than "since last time". A
// shelf id is random and minted at capture, so no commit older than the node
// can name it and a deep window adds no false positives; the bound exists only
// so a large history costs one bounded `git log`. Two hundred code commits is
// far more than lands between a merge and the sync that follows it. A finding
// left unrecorded for longer than that ages out of the window.
const unrecordedMergeScanLimit = 200

// UnrecordedMerge is one bug, chore, or tweak that commits on the integration
// branch name while the node has no recorded submission: it is still ready,
// in_progress, or check. Sync observes a shelf node's merge only from
// in_review, so without this finding the merge would pass in silence.
//
// It is an observation and nothing more. It moves no status, records no
// crossing, and writes no event, and it is recomputed from the tree and the
// integration branch on every read, so it stops on its own once the node is
// submitted, done, or cancelled.
type UnrecordedMerge struct {
	Noun   string
	ID     string
	Key    string
	Status string
	// Commits are the integration-branch commits whose messages name the node,
	// newest first. A commit that only records Tiller state (nothing outside
	// .tiller/ and artifacts/tiller/) is never among them.
	Commits []NamingCommit
}

// NamingCommit is one commit whose message names a shelf node: the full object
// id and the subject line.
type NamingCommit struct {
	Commit  string
	Subject string
}

// NextStep is the command the warning hands the reader. A merge that landed
// ahead of the ledger has no single-verb repair today: the node still walks its
// own start, check, and submit before sync can record it, and where it stands
// in that walk depends on its status and owner. `tiller status` is the typed
// oracle that names the node's next lawful move, so the hint points there.
//
// This is the one place the hint is written; every surface renders it from here.
func (m UnrecordedMerge) NextStep() string {
	return fmt.Sprintf("tiller status %s %s", m.Noun, m.ID)
}

// Warning is the whole warning as one sentence: the finding and the next step.
func (m UnrecordedMerge) Warning() string {
	return m.Finding() + "; next: " + m.NextStep()
}

// Finding says what was observed: the node, its status, and the newest commit
// that names it, with a count of any others.
func (m UnrecordedMerge) Finding() string {
	named := "a merged commit names it"
	if len(m.Commits) > 0 {
		newest := m.Commits[0]
		named = fmt.Sprintf("commit %s %q", shortCommit(newest.Commit), newest.Subject)
		if more := len(m.Commits) - 1; more > 0 {
			named += fmt.Sprintf(" and %d more name it", more)
		} else {
			named += " names it"
		}
	}
	return fmt.Sprintf("%s %s is %s with no recorded submission, but %s",
		m.Noun, gen.RefParts(m.Key, m.ID), m.Status, named)
}

// shortCommit abbreviates a full object id for a human line.
func shortCommit(commit string) string {
	const shortLen = 7
	if len(commit) <= shortLen {
		return commit
	}
	return commit[:shortLen]
}

// commitMessageReader is the one git read the scan makes. vcs.Revisions
// satisfies it.
type commitMessageReader interface {
	CodeCommitMessages(ref string, limit int) ([]vcs.CommitMessage, error)
}

// ObserveUnrecordedShelfMerges derives the finding outside a sync, for a
// surface that reports it later (the readout). It resolves the integration
// branch the way a flagless sync does, the manifest's git.integration_branch
// and then the branch HEAD is on, and reads that branch's tip.
//
// It runs no git at all while no shelf node is ready, in_progress, or check.
func ObserveUnrecordedShelfMerges(tree *store.Tree, rev vcs.Revisions) ([]UnrecordedMerge, error) {
	if len(unsubmittedShelfNodes(tree, "")) == 0 {
		return nil, nil
	}
	into, source, err := resolveInto(tree.Manifest, SyncInput{}, rev.HeadBranch)
	if err != nil {
		return nil, err
	}
	tip, err := resolveIntegrationTip(rev, into, source)
	if err != nil {
		return nil, err
	}
	return unrecordedShelfMerges(tree, rev, canonicalBranch(into, ""), tip.Commit)
}

// unrecordedShelfMerges scans the integration commit's recent history once and
// reports every unsubmitted shelf node a commit message names, shelf order then
// id order. intoBranch is the integration branch's own name and tipCommit the
// commit it resolved to.
func unrecordedShelfMerges(tree *store.Tree, rev commitMessageReader, intoBranch, tipCommit string) ([]UnrecordedMerge, error) {
	nodes := unsubmittedShelfNodes(tree, intoBranch)
	if len(nodes) == 0 {
		return nil, nil
	}
	landed, err := rev.CodeCommitMessages(tipCommit, unrecordedMergeScanLimit)
	if err != nil {
		return nil, err
	}
	var findings []UnrecordedMerge
	for _, node := range nodes {
		w := node.Body()
		var naming []NamingCommit
		for _, commit := range landed {
			if namesID(commit.Message, w.ID) {
				naming = append(naming, NamingCommit{Commit: commit.Commit, Subject: commit.Subject})
			}
		}
		if len(naming) == 0 {
			continue
		}
		findings = append(findings, UnrecordedMerge{
			Noun: node.Noun(), ID: w.ID, Key: deref(w.Key), Status: w.Status, Commits: naming,
		})
	}
	return findings, nil
}

// unsubmittedShelfNodes lists the shelf nodes the warning can be about: ready,
// in_progress, or check, with a merge of their own still to come.
//
// A carried node is left out, because its fix rides its carrier's merge and it
// never submits anything. A node holding a recovered review is left out too:
// its merge is on record and the next sync crosses it. So is a node whose
// working branch is the branch
// being read: a branch contains its own commits, and work in progress there has
// not landed anywhere.
func unsubmittedShelfNodes(tree *store.Tree, intoBranch string) []store.WorkNode {
	var nodes []store.WorkNode
	for _, noun := range store.ShelfNouns() {
		for _, node := range tree.WorkNodesOf(noun) {
			w := node.Body()
			if !awaitsSubmission(w.Status) || workNodeCarried(tree, node) || w.RecoveredReview != nil {
				continue
			}
			if intoBranch != "" && canonicalBranch(observationWorkBranch(tree.Manifest, node), "") == intoBranch {
				continue
			}
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// awaitsSubmission reports whether a shelf status is approved work that has not
// been submitted for review.
func awaitsSubmission(status string) bool {
	return status == "ready" || status == "in_progress" || status == "check"
}

// namesID reports whether message carries id as a whole token: not inside a
// longer run of letters and digits on either side.
func namesID(message, id string) bool {
	for from := 0; ; {
		at := strings.Index(message[from:], id)
		if at < 0 {
			return false
		}
		start := from + at
		end := start + len(id)
		if (start == 0 || !isIDByte(message[start-1])) && (end == len(message) || !isIDByte(message[end])) {
			return true
		}
		from = start + 1
	}
}

// isIDByte reports whether b can extend an id-like token.
func isIDByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}
