package mutate

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// ApproveDesign observes one project HEAD and one clock instant for the whole
// batch, then commits all stamps and events together. Working-tree/external
// observations are not locks on arbitrary editors or on Git refs.
func ApproveDesign(tree *store.Tree, deps Deps, docs []string) (DesignApprovalResult, error) {
	if len(docs) == 0 {
		return DesignApprovalResult{}, designPathError("design_approve", "approve design needs at least one named document")
	}
	state, err := readDesignState(tree, deps)
	if err != nil {
		return DesignApprovalResult{}, err
	}
	paths := make([]string, 0, len(docs))
	for _, raw := range docs {
		p, err := designDocPath(raw)
		if err != nil {
			return DesignApprovalResult{}, err
		}
		if slices.Contains(paths, p) {
			continue
		}
		if state.index(p) < 0 {
			return DesignApprovalResult{}, designPathError("design_unlinked", "design document %q has no describes registration", p)
		}
		if err := state.guardDoc(tree, p); err != nil {
			return DesignApprovalResult{}, err
		}
		paths = append(paths, p)
	}
	at := deps.clock().Now().UTC()
	head, err := deps.rev(tree.Root).CurrentCommit()
	if err != nil {
		return DesignApprovalResult{}, err
	}
	head.ObservedAt = at
	result := DesignApprovalResult{Docs: make([]DesignResult, 0, len(paths))}
	var evs []events.Event
	for _, p := range paths {
		index := state.index(p)
		doc := state.ledger.Docs[index]
		confirmation := &store.BaselineConfirmation{Commit: head, By: state.owner.ID, Surfaces: make([]store.SurfaceFingerprint, 0, len(doc.Describes))}
		for _, described := range doc.Describes {
			fingerprint, err := ObserveDesignSurface(tree.Root, state.manifest, described, at)
			if err != nil {
				return DesignApprovalResult{}, err
			}
			confirmation.Surfaces = append(confirmation.Surfaces, fingerprint)
		}
		changed := !sameDesignConfirmation(doc.Confirmed, confirmation)
		if !changed {
			changed, err = designCrossedAfterConfirmation(tree, deps.rev(tree.Root), doc)
			if err != nil {
				return DesignApprovalResult{}, err
			}
		}
		if changed {
			doc.Confirmed = confirmation
			state.ledger.Docs[index] = doc
			evs = append(evs, designEvent(deps, state.owner, "approve", doc, at, map[string]any{"commit": head.Value}))
		}
		result.Docs = append(result.Docs, DesignResult{Doc: doc, Changed: changed})
	}
	err = state.commit(tree, "approve.design", evs)
	return result, err
}

// Observation timestamps alone do not turn a retry into a new confirmation.
func sameDesignConfirmation(left, right *store.BaselineConfirmation) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.By != right.By || left.Commit.Kind != right.Commit.Kind || left.Commit.Value != right.Commit.Value || left.Commit.Path != right.Commit.Path || len(left.Surfaces) != len(right.Surfaces) {
		return false
	}
	for i, a := range left.Surfaces {
		b := right.Surfaces[i]
		if a.Path != b.Path || a.Kind != b.Kind || a.Value != b.Value || !slices.Equal(a.Inventory, b.Inventory) {
			return false
		}
	}
	return true
}

// A done crossing can be observed after approval at the same HEAD. Reapproval
// must advance that observation boundary even when the code bytes stayed equal.
//
// The decision reads the whole set of later overlapping crossings in feature-id
// order, never the first one a map range happens to visit. A known
// same-HEAD crossing wins. When no crossing is known-same but a historical
// comparison was unavailable, the human's current-state approval is recorded
// rather than refused: drift reports unavailable history on its own, and an
// unreachable old commit says nothing about the bytes being approved now.
func designCrossedAfterConfirmation(tree *store.Tree, rev vcs.Revisions, doc store.BaselineDoc) (bool, error) {
	if doc.Confirmed == nil {
		return false, nil
	}
	confirmed := doc.Confirmed.Commit
	described := DesignSurfaceCoordinates(tree.Root, tree.Manifest, doc.Describes)
	unavailable := false
	for _, featureID := range slices.Sorted(maps.Keys(tree.Features)) {
		feature := tree.Features[featureID]
		crossing := feature.VerifiedAgainst.Commit
		if feature.Status != "done" || !crossing.ObservedAt.After(confirmed.ObservedAt) {
			continue
		}
		if !designSurfacesOverlap(described, DesignSurfaceCoordinates(tree.Root, tree.Manifest, feature.Surface.Paths)) {
			continue
		}
		same, err := sameDesignCommit(rev, confirmed, crossing)
		if err != nil {
			unavailable = true
			continue
		}
		if same {
			return true, nil
		}
	}
	return unavailable, nil
}

func designSurfacesOverlap(described, surfaces []string) bool {
	for _, a := range described {
		for _, b := range surfaces {
			_, aContainsB := designWithin(filepath.Clean(a), filepath.Clean(b))
			_, bContainsA := designWithin(filepath.Clean(b), filepath.Clean(a))
			if aContainsB || bContainsA {
				return true
			}
		}
	}
	return false
}

// Git abbreviations depend on checkout configuration. Bidirectional ancestry
// proves identity without confusing an older or divergent crossing with HEAD.
func sameDesignCommit(rev vcs.Revisions, a, b store.RevisionRef) (bool, error) {
	if a.Kind != store.KindGitCommit || b.Kind != store.KindGitCommit || a.Value == "" || b.Value == "" {
		return false, nil
	}
	if a.Value == b.Value {
		return true, nil
	}
	left, right := strings.TrimPrefix(a.Value, "git:"), strings.TrimPrefix(b.Value, "git:")
	before, err := rev.IsMergedInto(left, right)
	if err != nil || !before {
		return false, err
	}
	return rev.IsMergedInto(right, left)
}
