package mutate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// layBug lays one proposed bug with a written contract and commits the
// tree, so approve has a blob to pin and start has a clean basis.
func layBug(t *testing.T, key string) string {
	t.Helper()
	root := writeOwnedRepo(t)
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "B" {
			return "B-aaaaaa"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: key, Title: "Bug " + key})
	require.NoError(t, err)
	writeBugContract(t, root, key, "## Reproduction\n\nReload mid-entry.\n\n## Fixed when\n\nValues survive a reload.\n")
	gitCommitAllM(t, root, "lay bug")
	return root
}

func writeBugContract(t *testing.T, root, key, body string) {
	t.Helper()
	p := filepath.Join(root, ".tiller", "bugs", key, "bug.md")
	require.NoError(t, os.WriteFile(p, []byte("# Bug "+key+"\n\n"+body), 0o644))
}

func TestApproveBugPinsContractAndStartRecordsBranch(t *testing.T) {
	t.Parallel()
	root := layBug(t, "blank-on-reload")
	res := driveTo(t, root, store.NounBug, "blank-on-reload", mutate.TransitionInput{Verb: "approve"})
	require.Equal(t, "ready", res.Status)
	b := load(t, root).Bugs["B-aaaaaa"]
	require.NotNil(t, b.AcceptedAgainst)
	require.Equal(t, store.KindGitBlob, b.AcceptedAgainst.Spec.Kind)
	require.Equal(t, filepath.ToSlash(filepath.Join(".tiller", "bugs", "blank-on-reload", "bug.md")), b.AcceptedAgainst.Spec.Path)

	// start needs the claim; an unclaimed bug refuses work_unclaimed.
	_, err := mutate.Transition(load(t, root), testDeps(), resolveNode(t, load(t, root), store.NounBug, "B-aaaaaa"), mutate.TransitionInput{Verb: "start"})
	var unclaimed *mutate.WorkUnclaimedError
	require.ErrorAs(t, err, &unclaimed)
	require.Equal(t, "tiller claim bug bugs/blank-on-reload", unclaimed.Seam)

	setWorkOwner(t, root, store.NounBug, "blank-on-reload", "jc")
	// start's own clock sits strictly after the add and approve events
	// above, both stamped at testDeps()'s shared canonical instant:
	// events.Sort ties same-instant events on their (random) EV id, so a
	// start.bug this test means to find as the tree's last event needs a
	// later instant to be deterministic rather than racing a string
	// compare (the same hazard add_workitem_test.go's layAttachTarget
	// documents for the identical reason).
	startDeps := testDeps()
	startDeps.Clock = fixedClock{t: time.Date(2026, 6, 27, 14, 31, 0, 0, time.UTC)}
	res = driveWith(t, root, startDeps, store.NounBug, "blank-on-reload", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, "in_progress", res.Status)
	require.Equal(t, "tiller/bugs/blank-on-reload", res.Branch)
	require.Equal(t, "tiller/bugs/blank-on-reload", load(t, root).Bugs["B-aaaaaa"].Branch)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Equal(t, "start.bug", evs[len(evs)-1].Type)
}

// setWorkOwner writes the owner field directly, the way a claim would, so
// the transition tests here do not depend on Task 6's claim verbs.
func setWorkOwner(t *testing.T, root, noun, key, owner string) {
	t.Helper()
	p := filepath.Join(root, store.ShelfRelDir(noun, key), store.ShelfStateFile(noun))
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, append(data, []byte("owner: "+owner+"\n")...), 0o644))
	gitCommitAllM(t, root, "claim "+key)
}

func TestStartBugRefusesOnContractDrift(t *testing.T) {
	t.Parallel()
	root := layBug(t, "alpha")
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "approve"})
	setWorkOwner(t, root, store.NounBug, "alpha", "jc")
	writeBugContract(t, root, "alpha", "## Reproduction\n\nEdited after triage.\n")
	_, err := mutate.Transition(load(t, root), testDeps(), resolveNode(t, load(t, root), store.NounBug, "B-aaaaaa"), mutate.TransitionInput{Verb: "start"})
	var drift *mutate.ActiveBasisError
	require.ErrorAs(t, err, &drift)
}

func TestReturnBugAndRefreshBasis(t *testing.T) {
	t.Parallel()
	root := layBug(t, "alpha")
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "approve"})
	writeBugContract(t, root, "alpha", "## Reproduction\n\nRevised.\n")
	res := driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "approve", RefreshBasis: true})
	require.Equal(t, "ready", res.Status)
	require.True(t, res.Changed)
	res = driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "return", Reason: "rework"})
	require.Equal(t, "proposed", res.Status)
}

func TestCancelBugRequiresAValidResolution(t *testing.T) {
	t.Parallel()
	root := layBug(t, "alpha")
	node := func() store.Node { return resolveNode(t, load(t, root), store.NounBug, "B-aaaaaa") }
	_, err := mutate.Transition(load(t, root), testDeps(), node(), mutate.TransitionInput{Verb: "cancel", Reason: "no"})
	require.Error(t, err)
	require.Equal(t, "resolution_required", reasonOf(t, err))
	_, err = mutate.Transition(load(t, root), testDeps(), node(), mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "fixed"})
	require.Equal(t, "invalid_resolution", reasonOf(t, err))
	_, err = mutate.Transition(load(t, root), testDeps(), node(), mutate.TransitionInput{Verb: "cancel", Reason: "dup", Resolution: "duplicate"})
	require.Equal(t, "duplicate_of_required", reasonOf(t, err))
	_, err = mutate.Transition(load(t, root), testDeps(), node(), mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "wontfix", DuplicateOf: "B-aaaaaa"})
	require.Equal(t, "duplicate_of_misplaced", reasonOf(t, err))

	// cancel's own clock sits strictly after layBug's add event, both
	// otherwise stamped at testDeps()'s shared canonical instant: the
	// identical determinism note on TestApproveBugPinsContractAndStartRecordsBranch.
	cancelDeps := testDeps()
	cancelDeps.Clock = fixedClock{t: time.Date(2026, 6, 27, 14, 31, 0, 0, time.UTC)}
	res := driveWith(t, root, cancelDeps, store.NounBug, "alpha", mutate.TransitionInput{Verb: "cancel", Reason: "not worth it", Resolution: "wontfix"})
	require.Equal(t, "cancelled", res.Status)
	b := load(t, root).Bugs["B-aaaaaa"]
	require.Equal(t, "wontfix", b.Resolution)
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	last := evs[len(evs)-1]
	require.Equal(t, "cancel.bug", last.Type)
	require.Equal(t, "wontfix", last.After["resolution"])
}

func TestCancelBugAsDuplicateNamesSurvivor(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	deps := testDeps()
	// The shared sequenceMinter mints every prefix from one list, including
	// EV, so a plain sequenceMinter("B-aaaaaa", "B-bbbbbb") would hand the
	// second event, not the second bug, the "B-bbbbbb" token. Reserving the
	// sequence to the "B" prefix alone, and letting every other prefix
	// (EV) mint for real, is the prefix-keyed closure layBug already uses
	// above.
	bugIDs := sequenceMinter("aaaaaa", "bbbbbb")
	deps.Mint = func(prefix string) string {
		if prefix == "B" {
			return bugIDs(prefix)
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "alpha", Title: "A"})
	require.NoError(t, err)
	_, err = mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "bravo", Title: "B"})
	require.NoError(t, err)
	_, err = mutate.Transition(load(t, root), testDeps(), resolveNode(t, load(t, root), store.NounBug, "bravo"),
		mutate.TransitionInput{Verb: "cancel", Reason: "same", Resolution: "duplicate", DuplicateOf: "bravo"})
	require.Equal(t, "duplicate_of_self", reasonOf(t, err))
	// cancel's own clock sits strictly after the two add events above, all
	// otherwise stamped at testDeps()'s shared canonical instant: the same
	// determinism note on TestApproveBugPinsContractAndStartRecordsBranch.
	cancelDeps := testDeps()
	cancelDeps.Clock = fixedClock{t: time.Date(2026, 6, 27, 14, 31, 0, 0, time.UTC)}
	res := driveWith(t, root, cancelDeps, store.NounBug, "bravo", mutate.TransitionInput{Verb: "cancel", Reason: "same", Resolution: "duplicate", DuplicateOf: "bugs/alpha"})
	require.Equal(t, "cancelled", res.Status)
	require.Equal(t, "B-aaaaaa", load(t, root).Bugs["B-bbbbbb"].DuplicateOf)

	// The event's own after.duplicate_of must carry the SAME resolved id
	// as the node record above, not the raw "bugs/alpha" key the caller
	// named: an audit trail built purely from the event ledger must never
	// disagree with the node record about what duplicate_of means
	// (round-1 review finding).
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	last := evs[len(evs)-1]
	require.Equal(t, "cancel.bug", last.Type)
	require.Equal(t, "B-aaaaaa", last.After["duplicate_of"])
}

func TestWorkItemIllegalVerbAnswersWithLegalNext(t *testing.T) {
	t.Parallel()
	root := layBug(t, "alpha")
	_, err := mutate.Transition(load(t, root), testDeps(), resolveNode(t, load(t, root), store.NounBug, "B-aaaaaa"), mutate.TransitionInput{Verb: "start"})
	var illegal *lifecycle.IllegalTransitionError
	require.ErrorAs(t, err, &illegal)
	require.Equal(t, store.NounBug, illegal.Noun)
	require.ElementsMatch(t, []string{"approve", "cancel"}, illegal.Legal)
}
