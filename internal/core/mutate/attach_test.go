package mutate_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// layShelfPair lays an epic, a feature, one bug, and one chore.
func layShelfPair(t *testing.T) string {
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
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "alpha", Title: "A"})
	require.NoError(t, err)
	_, err = mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounChore, Key: "bump", Title: "Bump"})
	require.NoError(t, err)
	return root
}

func TestLinkAndUnlinkShelfDependencies(t *testing.T) {
	t.Parallel()
	root := layShelfPair(t)
	res, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"CH-aaaaaa"}})
	require.NoError(t, err)
	require.Equal(t, []string{"CH-aaaaaa"}, res.DependsOn)
	require.Equal(t, "link.bug", lastEvent(t, root, "link.bug").Type)

	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"form-engine/loading"}})
	require.Error(t, err)
	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounFeature, Node: "form-engine/loading", DependsOn: []string{"B-aaaaaa"}})
	require.Error(t, err, "no tree node may depend on a bug")
	require.Contains(t, err.Error(), "not a dependency")

	un, err := mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"chores/bump"}})
	require.NoError(t, err)
	require.Equal(t, []string{"CH-aaaaaa"}, un.Removed)
	require.Empty(t, un.DependsOn)
}

func TestDependencyChangeLocksOnceABugStarts(t *testing.T) {
	t.Parallel()
	root := layShelfPair(t)
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "approve"})
	_, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounBug, "alpha", "")
	require.NoError(t, err)
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "start"})
	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"CH-aaaaaa"}})
	var locked *mutate.DependencyChangeLockedError
	require.ErrorAs(t, err, &locked)
	require.Equal(t, "in_progress", locked.Status)
}

func TestAttachAndDetachAtAnyState(t *testing.T) {
	t.Parallel()
	root := layShelfPair(t)
	res, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", Attach: "form-engine/loading"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "F-aaaaaa", res.AttachedTo)
	require.Equal(t, "F-aaaaaa", load(t, root).Bugs["B-aaaaaa"].AttachedTo)
	ev := lastEvent(t, root, "link.attach.bug")
	require.Equal(t, "F-aaaaaa", ev.After["attached_to"])

	again, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", Attach: "F-aaaaaa"})
	require.NoError(t, err)
	require.False(t, again.Changed)

	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", Attach: "CH-aaaaaa"})
	require.Equal(t, "attach_target", reasonOf(t, err))
	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", Attach: "  "})
	require.Equal(t, "attach_target", reasonOf(t, err), "a blank target refuses rather than detaching")
	require.Equal(t, "F-aaaaaa", load(t, root).Bugs["B-aaaaaa"].AttachedTo)

	// Attachment never gates: it moves on a cancelled bug too.
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "wontfix"})
	det, err := mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{Noun: store.NounBug, Node: "alpha", Detach: true})
	require.NoError(t, err)
	require.Equal(t, "F-aaaaaa", det.DetachedFrom)
	require.Equal(t, "", load(t, root).Bugs["B-aaaaaa"].AttachedTo)
	require.Equal(t, "unlink.attach.bug", lastEvent(t, root, "unlink.attach.bug").Type)
}

// A combined link plans dependency and attachment together: an unresolvable
// or illegal attachment leaves the valid dependency unwritten too.
func TestCombinedLinkRefusalWritesNothing(t *testing.T) {
	t.Parallel()
	root := layShelfPair(t)
	for _, attach := range []string{"missing", "CH-aaaaaa"} {
		before := snapshotDir(t, root)
		_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"CH-aaaaaa"}, Attach: attach})
		require.Error(t, err)
		requireNoWrites(t, before, snapshotDir(t, root))
	}
}

func TestCombinedLinkCommitsBothFactsTogether(t *testing.T) {
	t.Parallel()
	root := layShelfPair(t)
	before, err := events.List(load(t, root))
	require.NoError(t, err)
	res, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"CH-aaaaaa"}, Attach: "form-engine"})
	require.NoError(t, err)
	require.True(t, res.DependenciesChanged)
	require.True(t, res.AttachmentChanged)
	require.Equal(t, []string{"CH-aaaaaa"}, res.DependsOn)
	require.Equal(t, "E-aaaaaa", res.AttachedTo)
	bug := load(t, root).Bugs["B-aaaaaa"]
	require.Equal(t, []string{"CH-aaaaaa"}, bug.DependsOn)
	require.Equal(t, "E-aaaaaa", bug.AttachedTo)
	after, err := events.List(load(t, root))
	require.NoError(t, err)
	had := map[string]bool{}
	for _, ev := range before {
		had[ev.ID] = true
	}
	var types []string
	for _, ev := range after {
		if !had[ev.ID] {
			types = append(types, ev.Type)
		}
	}
	require.ElementsMatch(t, []string{"link.bug", "link.attach.bug"}, types)
}

// The dependency lock still applies to a combined link, and its refusal
// leaves the requested attachment unwritten.
func TestCombinedLinkLockedDependencyKeepsAttachment(t *testing.T) {
	t.Parallel()
	root := layShelfPair(t)
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "approve"})
	_, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounBug, "alpha", "")
	require.NoError(t, err)
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "start"})
	before := snapshotDir(t, root)
	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"CH-aaaaaa"}, Attach: "form-engine"})
	var locked *mutate.DependencyChangeLockedError
	require.ErrorAs(t, err, &locked)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestCombinedUnlinkRemovesBothFactsAndRepeatsAsNoOp(t *testing.T) {
	t.Parallel()
	root := layShelfPair(t)
	_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"CH-aaaaaa"}, Attach: "form-engine"})
	require.NoError(t, err)

	res, err := mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"chores/bump"}, Detach: true})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, []string{"CH-aaaaaa"}, res.Removed)
	require.Empty(t, res.DependsOn)
	require.Equal(t, "E-aaaaaa", res.DetachedFrom)
	bug := load(t, root).Bugs["B-aaaaaa"]
	require.Empty(t, bug.DependsOn)
	require.Empty(t, bug.AttachedTo)

	before := snapshotDir(t, root)
	again, err := mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"chores/bump"}, Detach: true})
	require.NoError(t, err)
	require.False(t, again.Changed)
	require.Empty(t, again.Removed)
	requireNoWrites(t, before, snapshotDir(t, root))
}

// A locked dependency removal refuses the whole unlink, so the attachment the
// same call asked to clear stays recorded.
func TestCombinedUnlinkLockedDependencyKeepsAttachment(t *testing.T) {
	t.Parallel()
	root := layShelfPair(t)
	_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"CH-aaaaaa"}, Attach: "form-engine"})
	require.NoError(t, err)
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "approve"})
	_, err = mutate.ClaimWork(load(t, root), testDeps(), store.NounBug, "alpha", "")
	require.NoError(t, err)
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "start"})

	before := snapshotDir(t, root)
	_, err = mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{Noun: store.NounBug, Node: "alpha", DependsOn: []string{"CH-aaaaaa"}, Detach: true})
	var locked *mutate.DependencyChangeLockedError
	require.ErrorAs(t, err, &locked)
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, "E-aaaaaa", load(t, root).Bugs["B-aaaaaa"].AttachedTo)
}
