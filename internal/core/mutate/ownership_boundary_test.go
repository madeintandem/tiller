package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func ownershipPlan(t *testing.T, grain string) (string, mutate.Deps, string, string) {
	t.Helper()
	deps := claimDeps()
	root, feature := layClaimFeature(t, deps)
	setIntegrationBranch(t, root, "main")
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: "project", ReviewGrain: grain})
	require.NoError(t, err)
	first, second := addTaskNamed(t, root, deps, "first"), addTaskNamed(t, root, deps, "second")
	writeFeatureProse(t, root)
	for _, key := range []string{"first", "second"} {
		writeTaskSpec(t, root, key, "## Approach\n\nImplement this task.")
	}
	driveWith(t, root, deps, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, deps, store.NounFeature, feature, mutate.TransitionInput{Verb: "spec"})
	driveWith(t, root, deps, store.NounFeature, feature, mutate.TransitionInput{Verb: "approve"})
	_, err = mutate.ClaimTask(load(t, root), deps, first, "")
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), second, "")
	require.NoError(t, err)
	return root, deps, first, second
}

func TestIndependentTaskOwnersStartAtEveryReviewGrain(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainTask, store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root, deps, first, second := ownershipPlan(t, grain)
			one := driveWith(t, root, deps, store.NounTask, first, mutate.TransitionInput{Verb: "start"})
			two := driveWith(t, root, depsAs(deps, "pat"), store.NounTask, second, mutate.TransitionInput{Verb: "start"})
			require.NotEqual(t, one.Branch, two.Branch)
			require.Equal(t, one.AggregateBranch, two.AggregateBranch)
			tree := load(t, root)
			require.Equal(t, "active", tree.Features["F-000001"].Status)
			require.True(t, tree.Tasks[first].Started)
			require.True(t, tree.Tasks[second].Started)
			require.Equal(t, "jc", tree.Tasks[first].Owner)
			require.Equal(t, "pat", tree.Tasks[second].Owner)
			require.Equal(t, []store.Contributor{{ID: "jc", Name: "JC Grubbs"}, {ID: "pat", Name: "Pat Nguyen"}}, tree.Contributors["F-000001"])
			before := snapshotDir(t, root)
			_, err := mutate.ReassignTask(tree, deps, second, "jc", "not an authorized continuation")
			var immutable *mutate.OwnerImmutableError
			require.ErrorAs(t, err, &immutable)
			_, err = mutate.ClaimTask(load(t, root), deps, second, "pat")
			require.ErrorAs(t, err, &immutable)
			_, err = mutate.ReleaseTask(load(t, root), depsAs(deps, "pat"), second)
			require.ErrorAs(t, err, &immutable)
			again, err := mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), second, "")
			require.NoError(t, err)
			require.False(t, again.Changed)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestFailedFirstStartLeavesNoActivationOrFrozenOwner(t *testing.T) {
	t.Parallel()
	for _, guard := range []string{"task branch collision", "feature dependency", "epic dependency", "parent question", "parent basis", "retired epic", "closed aggregate"} {
		t.Run(guard, func(t *testing.T) {
			root, deps, first, _ := ownershipPlan(t, store.ReviewGrainEpic)
			tree := load(t, root)
			feature, epic := tree.Features["F-000001"], tree.Epics["E-000001"]
			switch guard {
			case "task branch collision":
				gitRunM(t, root, "branch", "tiller/form-engine/certification-form-loading/first")
			case "feature dependency", "epic dependency":
				dependency, err := mutate.AddEpic(tree, deps, mutate.EpicInput{Key: "prerequisite", Title: "Prerequisite"})
				require.NoError(t, err)
				var doc any = feature
				path := filepath.Join(root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml")
				feature.DependsOn = []string{dependency.ID}
				if guard == "epic dependency" {
					epic.DependsOn = []string{dependency.ID}
					doc = epic
					path = filepath.Join(root, ".tiller/epics/form-engine/epic.yml")
				}
				data, err := yaml.Marshal(doc)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, data, 0o644))
			case "parent question":
				_, err := mutate.AddQuestion(tree, deps, mutate.QuestionInput{Title: "Contract", Statement: "Which contract?", Blocks: []string{feature.ID}})
				require.NoError(t, err)
			case "parent basis":
				writeFeatureProse(t, root)
				require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/form-engine/features/certification-form-loading/feature.md"), []byte("Changed contract"), 0o644))
			case "retired epic", "closed aggregate":
				epic.Status = "cancelled"
				if guard == "closed aggregate" {
					epic.Status = "done"
				}
				data, err := yaml.Marshal(epic)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/form-engine/epic.yml"), data, 0o644))
			}
			before := snapshotDir(t, root)
			err := tryTransition(t, root, deps, store.NounTask, first, mutate.TransitionInput{Verb: "start"})
			require.Error(t, err)
			requireNoWrites(t, before, snapshotDir(t, root))
			tree = load(t, root)
			require.False(t, tree.Tasks[first].Started)
			require.Empty(t, tree.Tasks[first].Branch)
			require.Equal(t, "ready", tree.Features[feature.ID].Status)
			require.Empty(t, tree.Features[feature.ID].Branch)
			require.Empty(t, tree.Epics[epic.ID].Branch)
		})
	}
}

func TestCancelledBeforeStartOwnershipIsHistory(t *testing.T) {
	t.Parallel()
	root, deps, first, _ := ownershipPlan(t, store.ReviewGrainTask)
	driveWith(t, root, deps, store.NounTask, first, mutate.TransitionInput{Verb: "cancel", Reason: "no longer needed"})
	require.False(t, load(t, root).Tasks[first].Started)
	before := snapshotDir(t, root)
	_, err := mutate.ReassignTask(load(t, root), deps, first, "pat", "relabel history")
	var immutable *mutate.OwnerImmutableError
	require.ErrorAs(t, err, &immutable)
	require.Empty(t, immutable.Seam, "terminal work cannot transfer")
	_, err = mutate.ReleaseTask(load(t, root), deps, first)
	require.ErrorAs(t, err, &immutable)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestShelfOwnerBoundarySurvivesReviewResumeAndCancellation(t *testing.T) {
	t.Parallel()
	for _, noun := range []string{store.NounBug, store.NounChore} {
		t.Run(noun, func(t *testing.T) {
			root, deps := writeTwoOwnerRepo(t), claimDeps()
			item, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: noun, Key: "work", Title: "Work"})
			require.NoError(t, err)
			_, err = mutate.ClaimWork(load(t, root), deps, noun, item.ID, "")
			require.NoError(t, err)
			driveWith(t, root, deps, noun, item.ID, mutate.TransitionInput{Verb: "approve"})
			driveWith(t, root, deps, noun, item.ID, mutate.TransitionInput{Verb: "start"})
			for _, verb := range []string{"start", "check", "submit", "resume", "cancel"} {
				if verb != "start" {
					in := mutate.TransitionInput{Verb: verb, Reason: "review continuation", Resolution: store.ResolutionWontfix}
					if verb == "submit" {
						pr := 41
						in.PR = &pr
					}
					driveWith(t, root, deps, noun, item.ID, in)
				}
				before := snapshotDir(t, root)
				_, err = mutate.ReassignWork(load(t, root), deps, noun, item.ID, "pat", "rewrite ownership")
				var immutable *mutate.OwnerImmutableError
				require.ErrorAs(t, err, &immutable)
				_, err = mutate.ClaimWork(load(t, root), depsAs(deps, "pat"), noun, item.ID, "jc")
				require.ErrorAs(t, err, &immutable)
				_, err = mutate.ReleaseWork(load(t, root), deps, noun, item.ID)
				require.ErrorAs(t, err, &immutable)
				requireNoWrites(t, before, snapshotDir(t, root))
			}
		})
	}
}

func TestThirdDeveloperSubmitsMixedOwnerAggregate(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			deps := claimDeps()
			root := rgLayFeature(t, deps, "pay", "checkout", "first", "second")
			for _, id := range []string{"pat", "reviewer"} {
				_, err := mutate.AddContributor(load(t, root), deps, store.Contributor{ID: id, Name: id})
				require.NoError(t, err)
			}
			_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: "project", ReviewGrain: grain})
			require.NoError(t, err)
			_, err = mutate.ReassignTask(load(t, root), deps, "pay/checkout/second", "pat", "independent implementation")
			require.NoError(t, err)
			rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "first", 1)
			rgDriveTaskToInReview(t, root, depsAs(deps, "pat"), "pay", "checkout", "second", 2)
			rgCheckFeatureGreen(t, root, depsAs(deps, "reviewer"), "pay", "checkout")
			noun, key := store.NounFeature, "pay/checkout"
			if grain == store.ReviewGrainEpic {
				noun, key = store.NounEpic, "pay"
			}
			pr := 42
			result, err := mutate.SubmitNode(load(t, root), depsAs(deps, "reviewer"), mutate.SubmitNodeInput{Noun: noun, Key: key, PR: &pr})
			require.NoError(t, err)
			require.Equal(t, &pr, result.PR)
			event := lastEvent(t, root, "submit."+noun)
			require.Equal(t, "reviewer", event.Contributor)
			for _, contributor := range load(t, root).Contributors["F-000001"] {
				require.NotEqual(t, "reviewer", contributor.ID)
			}
			before := snapshotDir(t, root)
			err = tryTransition(t, root, depsAs(deps, "reviewer"), store.NounTask, "pay/checkout/second", mutate.TransitionInput{Verb: "resume", Reason: "requested changes"})
			var held *mutate.WorkClaimHeldError
			require.ErrorAs(t, err, &held)
			require.Equal(t, "pat", held.Holder)
			requireNoWrites(t, before, snapshotDir(t, root))
			continued := driveWith(t, root, depsAs(deps, "pat"), store.NounTask, "pay/checkout/second", mutate.TransitionInput{Verb: "resume", Reason: "requested changes"})
			_, err = mutate.ReassignTask(load(t, root), deps, continued.ID, "jc", "cannot rewrite after resume")
			var immutable *mutate.OwnerImmutableError
			require.ErrorAs(t, err, &immutable)
			require.Equal(t, "pat", load(t, root).Tasks[continued.ID].Owner)
		})
	}
}

func TestIndependentOverrideStaysOutsideEpicAggregate(t *testing.T) {
	t.Parallel()
	deps := claimDeps()
	root := rgLayEpicWithTwoFeatures(t, deps, "pay")
	_, err := mutate.AddContributor(load(t, root), deps, store.Contributor{ID: "pat", Name: "Pat"})
	require.NoError(t, err)
	_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: store.NounFeature, Key: "pay/beta", ReviewGrain: store.ReviewGrainTask})
	require.NoError(t, err)
	_, err = mutate.ReassignTask(load(t, root), deps, "pay/beta/only-task", "pat", "independent review")
	require.NoError(t, err)
	alpha := driveWith(t, root, deps, store.NounTask, "pay/alpha/only-task", mutate.TransitionInput{Verb: "start"})
	require.NotEmpty(t, alpha.AggregateBranch)
	require.Equal(t, "ready", featureByKey(load(t, root), "pay/beta").Status, "starting one feature does not activate a sibling")
	beta := driveWith(t, root, depsAs(deps, "pat"), store.NounTask, "pay/beta/only-task", mutate.TransitionInput{Verb: "start"})
	require.Empty(t, beta.AggregateBranch)
	require.Empty(t, featureByKey(load(t, root), "pay/beta").Branch)
	pr := 42
	_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{Noun: store.NounEpic, Key: "pay", PR: &pr})
	var independent *mutate.FeatureNotSubmittableError
	require.ErrorAs(t, err, &independent)
	require.Equal(t, mutate.ReasonIndependentReviewOpen, independent.Reason)
}
