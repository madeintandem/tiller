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

// layTweak lays one proposed tweak with a written contract and commits the
// tree, so approve has a blob to pin and start has a clean basis.
func layTweak(t *testing.T, key string) string {
	t.Helper()
	root := writeOwnedRepo(t)
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "TW" {
			return "TW-aaaaaa"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounTweak, Key: key, Title: "Tweak " + key})
	require.NoError(t, err)
	writeTweakContract(t, root, key, "## Reproduction\n\nReload mid-entry.\n\n## Fixed when\n\nValues survive a reload.\n")
	gitCommitAllM(t, root, "lay tweak")
	return root
}

func writeTweakContract(t *testing.T, root, key, body string) {
	t.Helper()
	p := filepath.Join(root, ".tiller", "tweaks", key, "tweak.md")
	require.NoError(t, os.WriteFile(p, []byte("# Tweak "+key+"\n\n"+body), 0o644))
}

func TestApproveTweakPinsContractAndStartRecordsBranch(t *testing.T) {
	t.Parallel()
	root := layTweak(t, "blank-on-reload")
	res := driveTo(t, root, store.NounTweak, "blank-on-reload", mutate.TransitionInput{Verb: "approve"})
	require.Equal(t, "ready", res.Status)
	b := load(t, root).Tweaks["TW-aaaaaa"]
	require.NotNil(t, b.AcceptedAgainst)
	require.Equal(t, store.KindGitBlob, b.AcceptedAgainst.Spec.Kind)
	require.Equal(t, filepath.ToSlash(filepath.Join(".tiller", "tweaks", "blank-on-reload", "tweak.md")), b.AcceptedAgainst.Spec.Path)

	// start needs the claim; an unclaimed tweak refuses work_unclaimed.
	_, err := mutate.Transition(load(t, root), testDeps(), resolveNode(t, load(t, root), store.NounTweak, "TW-aaaaaa"), mutate.TransitionInput{Verb: "start"})
	var unclaimed *mutate.WorkUnclaimedError
	require.ErrorAs(t, err, &unclaimed)
	require.Equal(t, "tiller claim tweak tweaks/blank-on-reload", unclaimed.Seam)

	setTweakOwner(t, root, store.NounTweak, "blank-on-reload", "jc")
	// start's own clock sits strictly after the add and approve events
	// above, both stamped at testDeps()'s shared canonical instant:
	// events.Sort ties same-instant events on their (random) EV id, so a
	// start.tweak this test means to find as the tree's last event needs a
	// later instant to be deterministic rather than racing a string
	// compare (the same hazard add_workitem_test.go's layAttachTarget
	// documents for the identical reason).
	startDeps := testDeps()
	startDeps.Clock = fixedClock{t: time.Date(2026, 6, 27, 14, 31, 0, 0, time.UTC)}
	res = driveWith(t, root, startDeps, store.NounTweak, "blank-on-reload", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, "in_progress", res.Status)
	require.Equal(t, "tiller/tweaks/blank-on-reload", res.Branch)
	require.Equal(t, "tiller/tweaks/blank-on-reload", load(t, root).Tweaks["TW-aaaaaa"].Branch)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Equal(t, "start.tweak", evs[len(evs)-1].Type)
}

// setTweakOwner writes the owner field directly, the way a claim would, so
// the transition tests here do not depend on Task 6's claim verbs.
func setTweakOwner(t *testing.T, root, noun, key, owner string) {
	t.Helper()
	p := filepath.Join(root, store.ShelfRelDir(noun, key), store.ShelfStateFile(noun))
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, append(data, []byte("owner: "+owner+"\n")...), 0o644))
	gitCommitAllM(t, root, "claim "+key)
}

func TestStartTweakRefusesOnContractDrift(t *testing.T) {
	t.Parallel()
	root := layTweak(t, "alpha")
	driveTo(t, root, store.NounTweak, "alpha", mutate.TransitionInput{Verb: "approve"})
	setTweakOwner(t, root, store.NounTweak, "alpha", "jc")
	writeTweakContract(t, root, "alpha", "## Reproduction\n\nEdited after triage.\n")
	_, err := mutate.Transition(load(t, root), testDeps(), resolveNode(t, load(t, root), store.NounTweak, "TW-aaaaaa"), mutate.TransitionInput{Verb: "start"})
	var drift *mutate.ActiveBasisError
	require.ErrorAs(t, err, &drift)
}

func TestReturnTweakAndRefreshBasis(t *testing.T) {
	t.Parallel()
	root := layTweak(t, "alpha")
	driveTo(t, root, store.NounTweak, "alpha", mutate.TransitionInput{Verb: "approve"})
	writeTweakContract(t, root, "alpha", "## Reproduction\n\nRevised.\n")
	res := driveTo(t, root, store.NounTweak, "alpha", mutate.TransitionInput{Verb: "approve", RefreshBasis: true})
	require.Equal(t, "ready", res.Status)
	require.True(t, res.Changed)
	res = driveTo(t, root, store.NounTweak, "alpha", mutate.TransitionInput{Verb: "return", Reason: "rework"})
	require.Equal(t, "proposed", res.Status)
}

func TestCancelTweakRequiresAValidResolution(t *testing.T) {
	t.Parallel()
	root := layTweak(t, "alpha")
	node := func() store.Node { return resolveNode(t, load(t, root), store.NounTweak, "TW-aaaaaa") }
	_, err := mutate.Transition(load(t, root), testDeps(), node(), mutate.TransitionInput{Verb: "cancel", Reason: "no"})
	require.Error(t, err)
	require.Equal(t, "resolution_required", reasonOf(t, err))
	_, err = mutate.Transition(load(t, root), testDeps(), node(), mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "fixed"})
	require.Equal(t, "invalid_resolution", reasonOf(t, err))
	_, err = mutate.Transition(load(t, root), testDeps(), node(), mutate.TransitionInput{Verb: "cancel", Reason: "dup", Resolution: "duplicate"})
	require.Equal(t, "duplicate_of_required", reasonOf(t, err))
	_, err = mutate.Transition(load(t, root), testDeps(), node(), mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "wontfix", DuplicateOf: "TW-aaaaaa"})
	require.Equal(t, "duplicate_of_misplaced", reasonOf(t, err))

	// cancel's own clock sits strictly after layTweak's add event, both
	// otherwise stamped at testDeps()'s shared canonical instant: the
	// identical determinism note on TestApproveTweakPinsContractAndStartRecordsBranch.
	cancelDeps := testDeps()
	cancelDeps.Clock = fixedClock{t: time.Date(2026, 6, 27, 14, 31, 0, 0, time.UTC)}
	res := driveWith(t, root, cancelDeps, store.NounTweak, "alpha", mutate.TransitionInput{Verb: "cancel", Reason: "not worth it", Resolution: "wontfix"})
	require.Equal(t, "cancelled", res.Status)
	b := load(t, root).Tweaks["TW-aaaaaa"]
	require.Equal(t, "wontfix", b.Resolution)
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	last := evs[len(evs)-1]
	require.Equal(t, "cancel.tweak", last.Type)
	require.Equal(t, "wontfix", last.After["resolution"])
}

func TestCancelTweakAsDuplicateNamesSurvivor(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	deps := testDeps()
	// The shared sequenceMinter mints every prefix from one list, including
	// EV, so a plain sequenceMinter("TW-aaaaaa", "TW-bbbbbb") would hand the
	// second event, not the second tweak, the "TW-bbbbbb" token. Reserving the
	// sequence to the "TW" prefix alone, and letting every other prefix
	// (EV) mint for real, is the prefix-keyed closure layTweak already uses
	// above.
	tweakIDs := sequenceMinter("aaaaaa", "bbbbbb")
	deps.Mint = func(prefix string) string {
		if prefix == "TW" {
			return tweakIDs(prefix)
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounTweak, Key: "alpha", Title: "A"})
	require.NoError(t, err)
	_, err = mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounTweak, Key: "bravo", Title: "TW"})
	require.NoError(t, err)
	_, err = mutate.Transition(load(t, root), testDeps(), resolveNode(t, load(t, root), store.NounTweak, "bravo"),
		mutate.TransitionInput{Verb: "cancel", Reason: "same", Resolution: "duplicate", DuplicateOf: "bravo"})
	require.Equal(t, "duplicate_of_self", reasonOf(t, err))
	// cancel's own clock sits strictly after the two add events above, all
	// otherwise stamped at testDeps()'s shared canonical instant: the same
	// determinism note on TestApproveTweakPinsContractAndStartRecordsBranch.
	cancelDeps := testDeps()
	cancelDeps.Clock = fixedClock{t: time.Date(2026, 6, 27, 14, 31, 0, 0, time.UTC)}
	res := driveWith(t, root, cancelDeps, store.NounTweak, "bravo", mutate.TransitionInput{Verb: "cancel", Reason: "same", Resolution: "duplicate", DuplicateOf: "tweaks/alpha"})
	require.Equal(t, "cancelled", res.Status)
	require.Equal(t, "TW-aaaaaa", load(t, root).Tweaks["TW-bbbbbb"].DuplicateOf)

	// The event's own after.duplicate_of must carry the SAME resolved id
	// as the node record above, not the raw "tweaks/alpha" key the caller
	// named: an audit trail built purely from the event ledger must never
	// disagree with the node record about what duplicate_of means
	// (round-1 review finding).
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	last := evs[len(evs)-1]
	require.Equal(t, "cancel.tweak", last.Type)
	require.Equal(t, "TW-aaaaaa", last.After["duplicate_of"])
}

func TestTweakIllegalVerbAnswersWithLegalNext(t *testing.T) {
	t.Parallel()
	root := layTweak(t, "alpha")
	_, err := mutate.Transition(load(t, root), testDeps(), resolveNode(t, load(t, root), store.NounTweak, "TW-aaaaaa"), mutate.TransitionInput{Verb: "start"})
	var illegal *lifecycle.IllegalTransitionError
	require.ErrorAs(t, err, &illegal)
	require.Equal(t, store.NounTweak, illegal.Noun)
	require.ElementsMatch(t, []string{"approve", "cancel"}, illegal.Legal)
}

func TestTweakMixedShelfDependenciesAndOwnerHoldings(t *testing.T) {
	t.Parallel()
	root := layTweak(t, "alpha")
	deps := testDeps()
	eventIDs := sequenceMinter("dddddd", "eeeeee")
	deps.Mint = func(prefix string) string {
		if prefix == "EV" {
			return eventIDs(prefix)
		}
		return prefix + "-bbbbbb"
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounChore, Key: "housekeeping", Title: "Housekeeping"})
	require.NoError(t, err)
	_, err = mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "defect", Title: "Defect", DependsOn: []string{"tweaks/alpha", "chores/housekeeping"}})
	require.NoError(t, err)
	tree := load(t, root)
	require.Equal(t, []string{"TW-aaaaaa", "CH-bbbbbb"}, tree.Bugs["B-bbbbbb"].DependsOn)
	_, err = mutate.Link(tree, testDeps(), mutate.LinkInput{Noun: store.NounTweak, Node: "alpha", DependsOn: []string{"CH-bbbbbb"}})
	require.NoError(t, err)
	require.Equal(t, []string{"CH-bbbbbb"}, load(t, root).Tweaks["TW-aaaaaa"].DependsOn)
	_, err = mutate.ClaimWork(load(t, root), testDeps(), store.NounTweak, "alpha", "")
	require.NoError(t, err)
	_, err = mutate.AddContributor(load(t, root), testDeps(), store.Contributor{ID: "reviewer", Name: "Reviewer"})
	require.NoError(t, err)
	reviewer := testDeps()
	reviewer.Contributor = "reviewer"
	_, err = mutate.RemoveContributor(load(t, root), reviewer, "jc")
	var held *mutate.RemoveContributorHoldingsError
	require.ErrorAs(t, err, &held)
	require.Equal(t, 1, held.Holdings.Tweaks)
}

func TestTweakRejectsPlannedWorkDependenciesWithoutWrites(t *testing.T) {
	t.Parallel()
	root := layTweak(t, "alpha")
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "planned", Title: "Planned"})
	require.NoError(t, err)
	before := snapshotDir(t, root)
	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{Noun: store.NounTweak, Node: "alpha", DependsOn: []string{"planned"}})
	require.Error(t, err)
	requireNoWrites(t, before, snapshotDir(t, root))
}
