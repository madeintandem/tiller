package mutate

import (
	"path/filepath"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// observationWorkBranch resolves a shelf node's recorded working branch, with
// the existing shelf-only deterministic fallback for records predating the stamp.
func observationWorkBranch(m *store.Manifest, w store.WorkNode) string {
	b := w.Body()
	if b.Branch != "" {
		return b.Branch
	}
	if b.Key == nil {
		return ""
	}
	return branchFor(m, *b.Key)
}

// syncWorkItems is the third pass of Sync, after features and epics
// (docs/spec/cli-surface.md). For every
// shelf node, id order, bugs then chores:
//
//   - a node with a live carrier crosses on the carrier's done, basis
//     carried, read fresh off disk so a task or feature the two passes
//     above just crossed counts in this same run; a carrier not yet done
//     leaves the node where it is, and it is never listed as awaiting its
//     own merge, since it has none;
//   - a node whose carrier is cancelled, archived, or gone from the tree is
//     treated as having none for this pass, the same recompute the derived
//     carried phase makes (gen.WorkActivity): the merge it was waiting for
//     will never come, so parking it forever would strand it silently. The
//     resolved_by pointer is left on the record, because clearing or
//     re-pointing it is the human's judgment (`tiller resolve`), not
//     something an observation pass decides;
//   - an in_review node with no carrier, or none that can still carry it,
//     is observed on its own branch against the integration branch, exactly
//     as a feature branch is, and crosses on ancestry, content, or the
//     human's fast-forward assertion; unobserved, it is listed as awaiting.
//     A node in any other state simply waits for its own work.
//
// It computes its own awaiting entries; task observations are collected in the
// preceding pass, while each independent shelf node owns its own observation.
func syncWorkItems(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, tip integrationTip, cache *patchCache,
	attested map[string]mergeObs) ([]Crossing, []Awaiting, error) {
	fresh, err := store.Load(tree.Root)
	if err != nil {
		return nil, nil, err
	}
	var crossings []Crossing
	var awaiting []Awaiting
	for _, noun := range store.ShelfNouns() {
		for _, live := range tree.WorkNodesOf(noun) {
			w := live.Body()
			if w.Status == statusDone || w.Status == statusCancelled || w.Key == nil {
				continue
			}
			if workNodeCarried(fresh, live) {
				if carrierState(fresh, w.ResolvedBy) != statusDone {
					continue
				}
				c, cerr := crossWorkCarried(tree, deps, owner, live, tip.Ref)
				if cerr != nil {
					return nil, nil, cerr
				}
				crossings = append(crossings, c)
				continue
			}
			// Not carried, whether it never was or its carrier can no longer
			// carry it: the node answers on its own branch from here.
			if w.Status != "in_review" {
				continue
			}
			mo, already := attested[w.ID]
			if !already {
				o, oerr := observeMerge(rev, observationWorkBranch(tree.Manifest, live), tip.Commit, cache)
				if oerr != nil {
					return nil, nil, oerr
				}
				mo = o
			}
			if !mo.Merged {
				awaiting = append(awaiting, Awaiting{
					Noun: noun, ID: w.ID, Key: deref(w.Key), Branch: mo.Branch,
					BranchMissing: !mo.BranchExists, NoMergeWitness: mo.NoMergeWitness,
				})
				continue
			}
			match := ""
			if mo.Commit != "" {
				match = "git:" + mo.Commit
			}
			c, cerr := crossWorkDone(tree, deps, owner, live, tip.Ref, mo.Basis, match)
			if cerr != nil {
				return nil, nil, cerr
			}
			crossings = append(crossings, c)
		}
	}
	return crossings, awaiting, nil
}

// crossWorkDone records an observed merge on a shelf node: the lifecycle's
// own in_review -> done row, the basis, and any policy violations.
func crossWorkDone(tree *store.Tree, deps Deps, owner store.Contributor, live store.WorkNode, observed store.RevisionRef, basis, match string) (Crossing, error) {
	doc, ymlRel, snapshot, err := loadWorkItemFile(tree, live)
	if err != nil {
		return Crossing{}, err
	}
	w := doc.Body()
	before := w.Status
	if err := lifecycle.Apply(doc, "sync", lifecycle.Payload{}); err != nil {
		return Crossing{}, err
	}
	w.DoneBasis = basis
	violations := recordPolicyViolations(tree.Manifest, live.Noun(), &w.GatesBlock, observed)
	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return Crossing{}, err
	}
	if err := commitTransition(tree, deps, owner, TransitionInput{Verb: "sync", SyncBasis: basis, SyncMatch: match},
		live.Noun(), w.ID, ymlRel, ydata, snapshot, before, w.Status, ""); err != nil {
		return Crossing{}, err
	}
	live.Body().Status = w.Status
	return Crossing{Noun: live.Noun(), ID: w.ID, Key: deref(w.Key), Status: w.Status,
		Observed: observed.Value, Basis: basis, MatchedCommit: match, PolicyViolations: violations}, nil
}

// crossWorkCarried records the carried crossing: done set directly, the way
// a rollup sets it, because the observation belongs to the carrier and the
// lifecycle table deliberately carries no row for it. The event names the
// carrier beside the basis so an auditor can follow the merge that fixed it.
//
// It records no policy violations, which is the one way it differs from every
// other crossing in this file, and the omission is deliberate. A violation says
// "a gate this node's own work had to pass was not green at the merge", and a
// carried node never worked and never ran a gate: its fix rode in on the
// carrier, whose own crossing recorded the violations that merge really carried.
// Writing pending gates onto it would manufacture a finding out of a node that
// was never asked to answer for one.
func crossWorkCarried(tree *store.Tree, deps Deps, owner store.Contributor, live store.WorkNode, observed store.RevisionRef) (Crossing, error) {
	doc, ymlRel, snapshot, err := loadWorkItemFile(tree, live)
	if err != nil {
		return Crossing{}, err
	}
	w := doc.Body()
	before := w.Status
	w.Status = statusDone
	w.DoneBasis = basisCarried
	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return Crossing{}, err
	}
	ev := transitionEvent(deps, owner, TransitionInput{Verb: "sync", SyncBasis: basisCarried}, live.Noun(), w.ID, ymlRel, before, w.Status, "")
	ev.After["carrier"] = w.ResolvedBy
	if err := commitMutation(tree, "sync "+live.Noun(), []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, ymlRel), snapshot, ydata),
	}, []events.Event{ev}); err != nil {
		return Crossing{}, err
	}
	live.Body().Status = w.Status
	return Crossing{Noun: live.Noun(), ID: w.ID, Key: deref(w.Key), Status: w.Status, Observed: observed.Value, Basis: basisCarried}, nil
}
