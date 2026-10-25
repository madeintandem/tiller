package mutate_test

import (
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// layRemovalShelf lays a draft epic, a draft feature, a chore, and a bug
// attached to the feature, depending on the chore, and ordered.
func layRemovalShelf(t *testing.T) string {
	t.Helper()
	root := layAttachTarget(t)
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		switch prefix {
		case "B":
			return "B-aaaaaa"
		case "CH":
			return "CH-aaaaaa"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounChore, Key: "bump", Title: "Bump"})
	require.NoError(t, err)
	_, err = mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "alpha", Title: "A",
		AttachedTo: "form-engine/loading", DependsOn: []string{"CH-aaaaaa"}})
	require.NoError(t, err)
	_, err = mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{Noun: store.NounBug, Nodes: []string{"B-aaaaaa"}})
	require.NoError(t, err)
	return root
}

func TestRemoveProposedChoreScrubsShelfReferences(t *testing.T) {
	t.Parallel()
	root := layRemovalShelf(t)
	res, err := mutate.RemoveDraft(load(t, root), testDeps(), mutate.RemoveInput{Noun: store.NounChore, Node: "bump", Reason: "not needed"})
	require.NoError(t, err)
	require.Equal(t, []string{"CH-aaaaaa"}, res.RemovedIDs)
	require.NoDirExists(t, filepath.Join(root, ".tiller", "chores", "bump"))
	tree := load(t, root)
	require.Empty(t, tree.Chores)
	require.Empty(t, tree.Bugs["B-aaaaaa"].DependsOn, "the bug's edge to the removed chore is scrubbed")
	require.Equal(t, "remove.proposed.chore", lastEvent(t, root, "remove.proposed.chore").Type)
}

func TestRemoveFeatureClearsAttachmentOnSurvivingBug(t *testing.T) {
	t.Parallel()
	root := layRemovalShelf(t)
	res, err := mutate.RemoveDraft(load(t, root), testDeps(), mutate.RemoveInput{Noun: store.NounFeature, Node: "form-engine/loading", Reason: "dropped"})
	require.NoError(t, err)
	require.Equal(t, []string{"B-aaaaaa"}, res.Detached)
	require.Equal(t, "", load(t, root).Bugs["B-aaaaaa"].AttachedTo)
}

// TestRemoveClearsCarrierAndDuplicatePointers covers the other two provenance
// fields planRemovalShelfReferences scrubs, which only attached_to was pinned
// for: resolved_by, when the carrier a node was resolved by is removed, and
// duplicate_of, when the survivor a cancelled node was closed against is. Both
// name unapproved proposal material that never shipped, so the pointer goes
// with it rather than dangling, and each node whose field was cleared is
// reported in Detached.
func TestRemoveClearsCarrierAndDuplicatePointers(t *testing.T) {
	t.Parallel()
	root := layRemovalShelf(t)
	addShelfBug(t, root, "carried", "B-bbbbbb")
	addShelfBug(t, root, "survivor", "B-cccccc")
	addShelfBug(t, root, "dup", "B-dddddd")

	_, err := mutate.Resolve(load(t, root), testDeps(), mutate.ResolveInput{
		Noun: store.NounBug, Node: "carried", By: "form-engine/loading"})
	require.NoError(t, err)
	driveTo(t, root, store.NounBug, "dup", mutate.TransitionInput{
		Verb: "cancel", Reason: "already tracked", Resolution: store.ResolutionDuplicate, DuplicateOf: "B-cccccc"})
	require.Equal(t, "B-cccccc", load(t, root).Bugs["B-dddddd"].DuplicateOf)

	// Removing the draft feature takes both the bug attached to it and the bug
	// resolved by it, one field each.
	res, err := mutate.RemoveDraft(load(t, root), testDeps(), mutate.RemoveInput{
		Noun: store.NounFeature, Node: "form-engine/loading", Reason: "dropped"})
	require.NoError(t, err)
	require.Equal(t, []string{"B-aaaaaa", "B-bbbbbb"}, res.Detached)
	tree := load(t, root)
	require.Equal(t, "", tree.Bugs["B-aaaaaa"].AttachedTo)
	require.Equal(t, "", tree.Bugs["B-bbbbbb"].ResolvedBy)

	// Removing the survivor takes the duplicate pointer that named it.
	res, err = mutate.RemoveDraft(load(t, root), testDeps(), mutate.RemoveInput{
		Noun: store.NounBug, Node: "survivor", Reason: "never real"})
	require.NoError(t, err)
	require.Equal(t, []string{"B-dddddd"}, res.Detached)
	dup := load(t, root).Bugs["B-dddddd"]
	require.Equal(t, "", dup.DuplicateOf)
	require.Equal(t, store.ResolutionDuplicate, dup.Resolution,
		"the ruling that it was a duplicate stands; only the pointer to erased material goes")
}

// addShelfBug adds one bug under a forced id, for a fixture that needs more
// than the single bug layRemovalShelf lays.
func addShelfBug(t *testing.T, root, key, id string) {
	t.Helper()
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "B" {
			return id
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: key, Title: key})
	require.NoError(t, err)
}

func TestRemoveBugScrubsShelfOrderAndRefusesOnceReady(t *testing.T) {
	t.Parallel()
	root := layRemovalShelf(t)
	_, err := mutate.RemoveDraft(load(t, root), testDeps(), mutate.RemoveInput{Noun: store.NounBug, Node: "alpha", Reason: "dup"})
	require.NoError(t, err)
	require.Empty(t, load(t, root).BugOrder.Order)

	root = layRemovalShelf(t)
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "approve"})
	_, err = mutate.RemoveDraft(load(t, root), testDeps(), mutate.RemoveInput{Noun: store.NounBug, Node: "alpha", Reason: "dup"})
	var blocked *mutate.RemoveBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "remove_state_locked", blocked.Code)
}
