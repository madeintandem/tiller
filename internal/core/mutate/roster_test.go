package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// This file tests roster.go: AddOwner (moved here from the CLI so its event
// can ride the same events.NewEvent/deps.mintRaw path every other mutation's
// does) and RemoveOwner (new: the design's own census-guarded retirement).
// It reuses claim_test.go's twoOwnerManifest/writeTwoOwnerRepo/claimDeps/
// depsAs/layClaimFeature/lastEvent/snapshotDir/requireNoWrites/
// requireBlockedNotUsage fixtures, the same package's own shared idiom.

// --- AddOwner ---

func TestAddOwnerRegistersOwnerAndRecordsEvent(t *testing.T) {
	t.Parallel()
	root := writeTwoOwnerRepo(t)
	res, err := mutate.AddContributor(load(t, root), claimDeps(), store.Contributor{ID: "sam", Name: "Sam Okafor"})
	require.NoError(t, err)
	require.Equal(t, mutate.ContributorAddRegistered, res.Outcome)
	require.Equal(t, []store.Contributor{
		{ID: "jc", Name: "JC Grubbs"}, {ID: "pat", Name: "Pat Nguyen"}, {ID: "sam", Name: "Sam Okafor"},
	}, res.Roster)

	m, err := store.LoadManifest(root)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 3)

	ev := lastEvent(t, root, "contributor.add")
	require.Equal(t, "sam", ev.Contributor, "the event is about who joined, not a distinct actor: there is none for a bootstrap act")
	require.Contains(t, ev.Targets, "sam")
	require.Equal(t, "Sam Okafor", ev.After["name"])
	require.NotContains(t, ev.Before, "name", "a fresh registration carries no prior name")
	require.Equal(t, []events.FileChange{{Path: filepath.Join(".tiller", "tiller.yml")}}, ev.Files,
		"contributor.add names the manifest it touched")
}

func TestAddOwnerUpdatesNameAndRecordsEvent(t *testing.T) {
	t.Parallel()
	root := writeTwoOwnerRepo(t)
	res, err := mutate.AddContributor(load(t, root), claimDeps(), store.Contributor{ID: "pat", Name: "Patricia Nguyen"})
	require.NoError(t, err)
	require.Equal(t, mutate.ContributorAddNameUpdated, res.Outcome)

	m, err := store.LoadManifest(root)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 2, "a name update never appends a second row")
	require.Equal(t, "Patricia Nguyen", m.Contributors[1].Name)

	ev := lastEvent(t, root, "contributor.add")
	require.Equal(t, "Pat Nguyen", ev.Before["name"])
	require.Equal(t, "Patricia Nguyen", ev.After["name"])
}

func TestAddOwnerSameNameIsNoOpAndRecordsNoEvent(t *testing.T) {
	t.Parallel()
	root := writeTwoOwnerRepo(t)
	before := snapshotDir(t, root)

	res, err := mutate.AddContributor(load(t, root), claimDeps(), store.Contributor{ID: "pat", Name: "Pat Nguyen"})
	require.NoError(t, err)
	require.Equal(t, mutate.ContributorAddNoChange, res.Outcome)
	requireNoWrites(t, before, snapshotDir(t, root))
}

// --- RemoveOwner ---

func TestRemoveOwnerSucceedsAndRecordsEvent(t *testing.T) {
	t.Parallel()
	root := writeTwoOwnerRepo(t)
	res, err := mutate.RemoveContributor(load(t, root), depsAs(claimDeps(), "jc"), "pat")
	require.NoError(t, err)
	require.Equal(t, "pat", res.Removed.ID)
	require.Equal(t, []store.Contributor{{ID: "jc", Name: "JC Grubbs"}}, res.Roster)

	m, err := store.LoadManifest(root)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 1)
	require.Equal(t, "jc", m.Contributors[0].ID)

	ev := lastEvent(t, root, "contributor.remove")
	require.Equal(t, "jc", ev.Contributor, "the event's acting owner is who removed pat, not pat")
	require.Contains(t, ev.Targets, "pat")
	require.Equal(t, "Pat Nguyen", ev.Before["name"])
	require.Equal(t, []events.FileChange{{Path: filepath.Join(".tiller", "tiller.yml")}}, ev.Files,
		"contributor.remove names the manifest it touched")
}

func TestRemoveOwnerRefusesSelfRemoval(t *testing.T) {
	t.Parallel()
	root := writeTwoOwnerRepo(t)
	before := snapshotDir(t, root)

	_, err := mutate.RemoveContributor(load(t, root), depsAs(claimDeps(), "jc"), "jc")
	requireBlockedNotUsage(t, err)
	var self *mutate.RemoveContributorSelfError
	require.ErrorAs(t, err, &self)
	require.Equal(t, "jc", self.ID)
	require.Equal(t, mutate.ReasonRemoveContributorSelf, self.Reason)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestRemoveOwnerRefusesUnknownID(t *testing.T) {
	t.Parallel()
	root := writeTwoOwnerRepo(t)
	before := snapshotDir(t, root)

	_, err := mutate.RemoveContributor(load(t, root), depsAs(claimDeps(), "jc"), "nobody")
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	require.Equal(t, store.ReasonContributorUnknown, ee.Reason)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestRemoveOwnerRefusesUnregisteredActingOwner(t *testing.T) {
	t.Parallel()
	root := writeTwoOwnerRepo(t)
	_, err := mutate.RemoveContributor(load(t, root), depsAs(claimDeps(), "nobody"), "pat")
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	require.Equal(t, store.ReasonContributorUnknown, ee.Reason)
}

// Live task claims and open questions prevent removing their recorded owner.
func TestRemoveOwnerCensusBlocksOnLiveHoldings(t *testing.T) {
	t.Parallel()
	deps := claimDeps()
	root, _ := layClaimFeature(t, deps)
	taskID := addTaskNamed(t, root, deps, "transcribe-the-schema")
	_, err := mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), taskID, "")
	require.NoError(t, err)

	_, err = mutate.AddQuestion(load(t, root), depsAs(deps, "pat"), mutate.QuestionInput{Title: "Canonical schema version", Statement: "Which schema version?"})
	require.NoError(t, err)

	before := snapshotDir(t, root)
	_, err = mutate.RemoveContributor(load(t, root), depsAs(deps, "jc"), "pat")
	requireBlockedNotUsage(t, err)
	var held *mutate.RemoveContributorHoldingsError
	require.ErrorAs(t, err, &held)
	require.Equal(t, "pat", held.ID)
	require.Equal(t, mutate.ReasonRemoveContributorHoldings, held.Reason)
	require.Equal(t, 1, held.Holdings.Tasks)
	require.Equal(t, 1, held.Holdings.Questions)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestRemoveOwnerCensusIgnoresResolvedAndCancelledQuestions(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"resolved", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			root := writeTwoOwnerRepo(t)
			writeQuestionOwnedBy(t, root, "pat", status)

			res, err := mutate.RemoveContributor(load(t, root), depsAs(claimDeps(), "jc"), "pat")
			require.NoError(t, err)
			require.Equal(t, "pat", res.Removed.ID)
		})
	}
}

func TestRemoveOwnerCensusIgnoresDoneAndCancelledTasks(t *testing.T) {
	t.Parallel()
	deps := claimDeps()
	root, _ := layClaimFeature(t, deps)
	doneID := addTaskNamed(t, root, deps, "already-shipped")
	cancelledID := addTaskNamed(t, root, deps, "dropped-in-review")
	_, err := mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), doneID, "")
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), cancelledID, "")
	require.NoError(t, err)
	setTaskStatusOnDisk(t, root, doneID, "done")
	setTaskStatusOnDisk(t, root, cancelledID, "cancelled")

	res, err := mutate.RemoveContributor(load(t, root), depsAs(deps, "jc"), "pat")
	require.NoError(t, err, "pat's only remaining stamps are on a done and a cancelled task, neither live")
	require.Equal(t, "pat", res.Removed.ID)
}

func TestRemoveOwnerSucceedsAfterTaskReassignment(t *testing.T) {
	t.Parallel()
	deps := claimDeps()
	root, _ := layClaimFeature(t, deps)
	taskID := addTaskNamed(t, root, deps, "transcribe-the-schema")
	_, err := mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), taskID, "")
	require.NoError(t, err)

	_, err = mutate.RemoveContributor(load(t, root), deps, "pat")
	var held *mutate.RemoveContributorHoldingsError
	require.ErrorAs(t, err, &held)
	require.Equal(t, 1, held.Holdings.Tasks)

	_, err = mutate.ReassignTask(load(t, root), deps, taskID, "jc", "JC takes this unstarted task")
	require.NoError(t, err)
	_, err = mutate.RemoveContributor(load(t, root), deps, "pat")
	require.NoError(t, err)
	require.Equal(t, "jc", load(t, root).Tasks[taskID].Owner)
}

// TestRemoveOwnerCensusCountsShelfClaims pins the shelf half of the census. A
// bug or chore carries its own claim, so an owner holding one is holding live
// work and `owner remove` has to refuse for the same reason a live feature
// claim refuses. Without it the roster row goes while `owner: pat` stays on
// the node, and the shelf work gate then trusts an id the roster cannot
// resolve.
func TestRemoveOwnerCensusCountsShelfClaims(t *testing.T) {
	t.Parallel()
	deps := claimDeps()
	root := writeTwoOwnerRepo(t)
	_, err := mutate.AddWorkItem(load(t, root), depsAs(deps, "jc"), mutate.WorkItemInput{Noun: store.NounBug, Key: "alpha", Title: "Alpha"})
	require.NoError(t, err)
	_, err = mutate.AddWorkItem(load(t, root), depsAs(deps, "jc"), mutate.WorkItemInput{Noun: store.NounChore, Key: "bump", Title: "Bump"})
	require.NoError(t, err)
	_, err = mutate.ClaimWork(load(t, root), depsAs(deps, "pat"), store.NounBug, "alpha", "")
	require.NoError(t, err)
	_, err = mutate.ClaimWork(load(t, root), depsAs(deps, "pat"), store.NounChore, "bump", "")
	require.NoError(t, err)

	before := snapshotDir(t, root)
	_, err = mutate.RemoveContributor(load(t, root), depsAs(deps, "jc"), "pat")
	requireBlockedNotUsage(t, err)
	var held *mutate.RemoveContributorHoldingsError
	require.ErrorAs(t, err, &held)
	require.Equal(t, mutate.ReasonRemoveContributorHoldings, held.Reason)
	require.Equal(t, 1, held.Holdings.Bugs)
	require.Equal(t, 1, held.Holdings.Chores)
	require.Contains(t, held.Error(), "live bug")
	require.Contains(t, held.Error(), "live chore")
	requireNoWrites(t, before, snapshotDir(t, root))

	// Released, the same owner comes off the roster: only LIVE holdings block.
	_, err = mutate.ReleaseWork(load(t, root), depsAs(deps, "pat"), store.NounBug, "alpha")
	require.NoError(t, err)
	_, err = mutate.ReleaseWork(load(t, root), depsAs(deps, "pat"), store.NounChore, "bump")
	require.NoError(t, err)
	res, err := mutate.RemoveContributor(load(t, root), depsAs(deps, "jc"), "pat")
	require.NoError(t, err)
	require.Equal(t, "pat", res.Removed.ID)
}

func writeQuestionOwnedBy(t *testing.T, root, owner, status string) {
	t.Helper()
	questions := store.QuestionsFile{Questions: []*store.Question{{
		ID:        "Q-zzzzzz",
		Statement: "Which schema version?",
		Owner:     owner,
		Status:    status,
	}}}
	out, err := yaml.Marshal(questions)
	require.NoError(t, err)
	dir := filepath.Join(root, ".tiller", "decisions")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "open-questions.yml"), out, 0o644))
}
