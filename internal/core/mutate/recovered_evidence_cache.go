package mutate

import (
	"sync"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// A recovered review's evidence is immutable commits checked against
// integration's first-parent history, so a check that passed holds until
// integration moves. tiller serve observes workflow on every read; this lets
// those reads reuse a pass for the same record and integration tip instead of
// rerunning about ten git processes. Only passes are kept, so a fetch or
// restore that fixes a refusal shows on the next read. Sync, transfer, and
// recovery always validate directly. A cached pass assumes the record's commits
// stay present, so if a reviewed head is later pruned, tiller serve keeps the
// pass until integration moves or the process restarts, while sync, transfer,
// and recovery still refuse with object_unavailable.
const verifiedEvidenceLimit = 64

type evidenceKey struct {
	root, into, tip, recordInto, branch, basis string
	head, merge                                store.RevisionRef
}

type evidenceCache struct {
	mu      sync.Mutex
	limit   int
	entries map[evidenceKey]bool
}

var verifiedEvidence = &evidenceCache{limit: verifiedEvidenceLimit, entries: map[evidenceKey]bool{}}

// check runs validate unless key already passed, remembering a pass.
func (c *evidenceCache) check(key evidenceKey, validate func() error) error {
	c.mu.Lock()
	passed := c.entries[key]
	c.mu.Unlock()
	if passed {
		return nil
	}
	if err := validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.limit {
		clear(c.entries)
	}
	c.entries[key] = true
	return nil
}

// checkRecoveredEvidence is validateRecoveredEvidence for read-side workflow
// observation, reusing a pass for the same record and integration tip.
func checkRecoveredEvidence(revision *vcs.Git, root string, record *store.RecoveredReview, into string) error {
	tip := ""
	if into != "" {
		var err error
		if tip, err = revision.RefCommit(branchRef(into)); err != nil {
			return err
		}
	}
	validate := func() error {
		_, err := validateRecoveredEvidence(revision, record, into, tip)
		return err
	}
	if tip == "" {
		return validate()
	}
	key := evidenceKey{root: root, into: into, tip: tip, recordInto: record.IntegrationBranch,
		branch: record.Branch, basis: record.Basis, head: record.Head, merge: record.MergeCommit}
	return verifiedEvidence.check(key, validate)
}
