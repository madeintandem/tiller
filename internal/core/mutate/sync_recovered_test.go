package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// Recovery is record-only; these fixtures pin an already integrated review so
// Sync's consumers can be exercised independently of the recovery command.
func recoveredSyncFixture(t *testing.T, grain, shape string, splitAcrossFeatures ...bool) (string, *store.RecoveredReview) {
	t.Helper()
	root := independentSyncBranches(t)
	declareIntegrationBranch(t, root)
	branch := recordSyncAggregate(t, root, grain, false)
	if len(splitAcrossFeatures) > 0 && splitAcrossFeatures[0] {
		require.Equal(t, store.ReviewGrainEpic, grain)
		splitRecoveredFixtureFeatures(t, root)
	}
	tree := load(t, root)
	tree.Manifest.Contributors = append(tree.Manifest.Contributors, store.Contributor{ID: "sam", Name: "Sam"})
	transferWrite(t, root, ".tiller/tiller.yml", tree.Manifest)
	for _, task := range tree.Tasks {
		task.Gates["tests"] = store.Gate{Status: store.GateStatusPending}
		task.Gates["build"] = store.Gate{Status: store.GateStatusPass}
		task.Gates["evals"] = store.Gate{Status: store.GateStatusPass}
		rel, err := filepath.Rel(root, taskYMLPath(t, root, task.ID))
		require.NoError(t, err)
		transferWrite(t, root, rel, task)
	}
	feature := tree.Features["F-aaaaaa"]
	for _, current := range tree.Features {
		if current.Gates == nil {
			current.Gates = map[string]store.Gate{}
		}
		current.Gates["evals"] = store.Gate{Status: store.GateStatusPending, Criteria: map[string]store.Criterion{"C8": {Status: "needs_attestation"}}}
		rel := filepath.Join(".tiller", "epics", "form-engine", "features", filepath.Base(*current.Key), "feature.yml")
		transferWrite(t, root, rel, current)
	}
	gitCommitAllM(t, root, "record submitted task state and historical acceptance")
	gitRunM(t, root, "checkout", "-b", branch, "main")
	for _, taskBranch := range []string{syncFirstTaskBranch, syncSecondTaskBranch} {
		gitRunM(t, root, "merge", "--no-ff", "-m", "assemble reviewed work", taskBranch)
	}
	head := gitOutM(t, root, "rev-parse", "HEAD")
	gitRunM(t, root, "checkout", "main")
	if shape == "contained" {
		gitRunM(t, root, "merge", "--ff-only", branch)
	} else {
		mergeSyncBranch(t, root, branch, shape)
	}
	result := gitOutM(t, root, "rev-parse", "HEAD")
	basis := "ancestry"
	if shape == "squash" {
		basis = "content"
	} else if shape == "contained" {
		basis = "contained"
	}
	rev := vcs.New(root)
	record := &store.RecoveredReview{
		Branch: branch, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + head},
		MergeCommit:       store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + result},
		IntegrationBranch: "main", Basis: basis, RecoveredBy: "jc", RecoveredAt: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC), Reason: "Authorized historical review bookkeeping",
	}
	for _, id := range []string{"T-aaaaaa", "T-aaaaab"} {
		path, err := filepath.Rel(root, taskYMLPath(t, root, id))
		require.NoError(t, err)
		_, pin, err := rev.ReadFileAt(head, filepath.ToSlash(path))
		require.NoError(t, err)
		record.Tasks = append(record.Tasks, store.RecoveredTask{ID: id, Path: filepath.ToSlash(path), State: pin})
	}
	policyPath := ".tiller/epics/form-engine/features/certification-form-loading/feature.yml"
	_, manifestPin, err := rev.ReadFileAt(head, ".tiller/tiller.yml")
	require.NoError(t, err)
	record.PolicySnapshots = []store.HistoricalPolicySnapshot{{Path: ".tiller/tiller.yml", Source: manifestPin}}
	for _, featureID := range []string{"F-aaaaaa", "F-aaaaab"} {
		current := tree.Features[featureID]
		if current == nil {
			continue
		}
		path := filepath.ToSlash(filepath.Join(".tiller", "epics", "form-engine", "features", filepath.Base(*current.Key), "feature.yml"))
		_, policyPin, err := rev.ReadFileAt(head, path)
		require.NoError(t, err)
		record.PolicySnapshots = append(record.PolicySnapshots, store.HistoricalPolicySnapshot{
			NodeID: featureID, Path: path, Source: policyPin,
			Observations: []store.HistoricalPolicyObservation{{Gate: "evals", Status: "pending", UnresolvedCriteria: []string{"C8"}, EvidenceRefs: []string{"historical:C8"}}},
		})
	}
	for _, task := range record.Tasks {
		record.PolicySnapshots = append(record.PolicySnapshots, store.HistoricalPolicySnapshot{NodeID: task.ID, Path: task.Path, Source: task.State, Observations: []store.HistoricalPolicyObservation{{Gate: "tests", Status: "pending"}}})
	}
	// Current acceptance is independently green and no longer contains C8.
	tree = load(t, root)
	for _, task := range tree.Tasks {
		task.Gates["tests"] = store.Gate{Status: store.GateStatusPass}
		task.Gates["build"] = store.Gate{Status: store.GateStatusPass}
		task.Gates["evals"] = store.Gate{Status: store.GateStatusPass}
		rel, err := filepath.Rel(root, taskYMLPath(t, root, task.ID))
		require.NoError(t, err)
		transferWrite(t, root, rel, task)
	}
	feature = tree.Features["F-aaaaaa"]
	for _, current := range tree.Features {
		current.Gates["evals"] = store.Gate{Status: store.GateStatusPass}
		current.Gates["build"] = store.Gate{Status: store.GateStatusPass}
		current.Gates["tests"] = store.Gate{Status: store.GateStatusPass}
		rel := filepath.Join(".tiller", "epics", "form-engine", "features", filepath.Base(*current.Key), "feature.yml")
		transferWrite(t, root, rel, current)
	}
	if grain == store.ReviewGrainFeature {
		feature.ReviewRef = &store.ReviewRef{Branch: branch, Head: record.Head}
		feature.RecoveredReview = record
	} else {
		epic := tree.Epics["E-aaaaaa"]
		epic.ReviewRef = &store.ReviewRef{Branch: branch, Head: record.Head}
		epic.RecoveredReview = record
		transferWrite(t, root, ".tiller/epics/form-engine/epic.yml", epic)
	}
	transferWrite(t, root, policyPath, feature)
	ownerID := "F-aaaaaa"
	if grain == store.ReviewGrainEpic {
		ownerID = "E-aaaaaa"
	}
	event := events.Event{ID: "EV-recovered", Type: "recover_review." + grain, At: record.RecoveredAt, Contributor: record.RecoveredBy,
		Command: "authorized historical review recovery", Targets: []string{ownerID}, After: map[string]any{"recovered_review": record}}
	eventPath, eventData, err := events.File(load(t, root), event)
	require.NoError(t, err)
	eventRel, err := filepath.Rel(root, eventPath)
	require.NoError(t, err)
	transferBytes(t, root, eventRel, eventData)
	gitCommitAllM(t, root, "record recovery and independent current green acceptance")
	return root, record
}

func TestRecoveredSyncUsesFrozenReviewWithMovedOrDeletedRefs(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		for _, shape := range []string{"merge", "squash"} {
			for _, refs := range []string{"advanced_aggregate", "deleted_aggregate_and_tasks"} {
				t.Run(grain+"/"+shape+"/"+refs, func(t *testing.T) {
					root, record := recoveredSyncFixture(t, grain, shape)
					if refs == "advanced_aggregate" {
						gitRunM(t, root, "branch", "-f", record.Branch, "main")
						gitRunM(t, root, "checkout", record.Branch)
						commitFile(t, root, "post-review.txt", "not part of the integrated review")
						gitRunM(t, root, "checkout", "main")
					} else {
						gitRunM(t, root, "branch", "-D", record.Branch, syncFirstTaskBranch, syncSecondTaskBranch)
					}
					res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
					require.NoError(t, err)
					require.Equal(t, []string{"T-aaaaaa", "T-aaaaab", "F-aaaaaa", "E-aaaaaa"}, crossedIDs(res))
					for _, crossing := range res.Crossings {
						require.Empty(t, crossing.PolicyViolations, "green current gates must not inherit historical nonpassing gate names")
					}
					for _, crossing := range res.Crossings[:3] {
						require.Len(t, crossing.HistoricalPolicyViolations, 1)
						require.Equal(t, store.GateStatusPending, crossing.HistoricalPolicyViolations[0].Status)
						require.NotEmpty(t, crossing.HistoricalPolicyViolations[0].SourceRevision.Value)
					}
					require.Equal(t, "evals", res.Crossings[2].HistoricalPolicyViolations[0].Gate)
					require.Equal(t, []string{"C8"}, res.Crossings[2].HistoricalPolicyViolations[0].UnresolvedCriteria)
					tree := load(t, root)
					f := tree.Features["F-aaaaaa"]
					require.Equal(t, store.GateStatusPass, f.Gates["evals"].Status)
					require.NotContains(t, f.Gates["evals"].Criteria, "C8")
					require.Len(t, f.PolicyViolations, 1)
					require.Equal(t, []string{"C8"}, f.PolicyViolations[0].UnresolvedCriteria)
					require.Equal(t, record.PolicySnapshots[1].Source.Value, f.PolicyViolations[0].SourceRevision.Value)
					for _, task := range tree.Tasks {
						require.Len(t, task.PolicyViolations, 1)
						require.NotEmpty(t, task.PolicyViolations[0].SourceRevision.Value)
					}
					before := snapshotDir(t, root)
					again, err := mutate.Sync(tree, syncDeps(), vcs.New(root), mutate.SyncInput{})
					require.NoError(t, err)
					require.Empty(t, again.Crossings)
					requireNoWrites(t, before, snapshotDir(t, root))
				})
			}
		}
	}
}

func TestRecoveredSyncRefusesStaleEvidenceWithoutCrossings(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"alternate_into", "force_pushed_result", "force_pushed_result_with_tag", "advanced_task", "changed_submission", "changed_task_scope", "changed_owner", "task_owned_pr", "changed_grain", "new_rider", "frozen_blob_mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			root, record := recoveredSyncFixture(t, store.ReviewGrainFeature, "merge")
			in := mutate.SyncInput{}
			switch scenario {
			case "alternate_into":
				gitRunM(t, root, "branch", "release", "main")
				in.Into = "release"
			case "force_pushed_result", "force_pushed_result_with_tag":
				if scenario == "force_pushed_result_with_tag" {
					gitRunM(t, root, "tag", "main", record.MergeCommit.Value[4:])
				}
				gitRunM(t, root, "update-ref", "refs/heads/main", gitOutM(t, root, "rev-parse", record.MergeCommit.Value[4:]+"^1"))
			case "advanced_task":
				gitRunM(t, root, "checkout", syncSecondTaskBranch)
				commitFile(t, root, "late-task.txt", "not reviewed")
				gitRunM(t, root, "checkout", "main")
			case "frozen_blob_mismatch":
				f := load(t, root).Features["F-aaaaaa"]
				f.RecoveredReview.Tasks[1].State.Value = record.Tasks[0].State.Value
				f.RecoveredReview.PolicySnapshots[3].Source.Value = record.Tasks[0].State.Value
				transferWrite(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml", f)
			case "changed_grain":
				f := load(t, root).Features["F-aaaaaa"]
				f.ReviewGrain = store.ReviewGrainTask
				transferWrite(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml", f)
			case "new_rider":
				deps := seqDeps()
				rgAddTask(t, root, deps, "form-engine", "certification-form-loading", "later-work")
				driveWith(t, root, deps, store.NounTask, "form-engine/certification-form-loading/later-work", mutate.TransitionInput{Verb: "approve"})
			default:
				task := load(t, root).Tasks["T-aaaaab"]
				switch scenario {
				case "changed_submission":
					task.ReviewRef.Head = record.Head
				case "changed_task_scope":
					task.AcceptedAgainst.Spec.Value = "git:" + gitOutM(t, root, "rev-parse", "HEAD:first.txt")
				case "changed_owner":
					task.Owner = "sam"
				case "task_owned_pr":
					pr := 42
					task.PR = &pr
				}
				path, err := filepath.Rel(root, taskYMLPath(t, root, "T-aaaaab"))
				require.NoError(t, err)
				transferWrite(t, root, path, task)
			}
			before := snapshotDir(t, root)
			tree, err := store.Load(root)
			if scenario == "changed_grain" {
				require.Error(t, err, "malformed recovered ownership must refuse at the schema boundary, before ordinary sync can be selected")
				requireNoWrites(t, before, snapshotDir(t, root))
				return
			}
			require.NoError(t, err)
			_, err = mutate.Sync(tree, syncDeps(), vcs.New(root), in)
			require.Error(t, err)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestRecoveredContainedReviewRequiresExplicitHumanAssertion(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root, record := recoveredSyncFixture(t, grain, "contained")
			gitRunM(t, root, "branch", "-D", record.Branch, syncFirstTaskBranch, syncSecondTaskBranch)
			before := snapshotDir(t, root)
			res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Empty(t, res.Crossings)
			require.Len(t, res.Awaiting, 2)
			for _, pending := range res.Awaiting {
				require.True(t, pending.NoMergeWitness)
				require.False(t, pending.BranchMissing, "immutable reviewed head is available")
			}
			requireNoWrites(t, before, snapshotDir(t, root))
			id := "F-aaaaaa"
			if grain == store.ReviewGrainEpic {
				id = "E-aaaaaa"
			}
			res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{id}})
			require.NoError(t, err)
			require.Len(t, res.Crossings, 4)
			for _, taskID := range []string{"T-aaaaaa", "T-aaaaab"} {
				require.Equal(t, "human_attested", taskDoneBasis(t, root, taskID))
			}
		})
	}
}

func TestRecoveredReviewCannotHideIntegratedWorkFromTransfer(t *testing.T) {
	t.Parallel()
	for _, refs := range []string{"deleted", "moved"} {
		t.Run(refs, func(t *testing.T) {
			root, record := recoveredSyncFixture(t, store.ReviewGrainFeature, "squash")
			f := load(t, root).Features["F-aaaaaa"]
			pr := 41
			f.PR, f.ReviewRef = &pr, nil
			transferWrite(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml", f)
			if refs == "deleted" {
				gitRunM(t, root, "branch", "-D", record.Branch, syncFirstTaskBranch, syncSecondTaskBranch)
			} else {
				gitRunM(t, root, "branch", "-f", record.Branch, "main^1")
			}
			before := snapshotDir(t, root)
			_, err := mutate.Transfer(load(t, root), syncDeps(), mutate.TransferInput{Noun: store.NounTask, Node: "T-aaaaaa", To: "sam", Reason: "human-directed transfer"})
			var refusal *mutate.TransferBlockedError
			require.ErrorAs(t, err, &refusal)
			require.Equal(t, "transfer_review_merged", refusal.Code)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestRecoveredSyncKeepsHistoricalAndCurrentPolicySeparate(t *testing.T) {
	t.Parallel()
	root, _ := recoveredSyncFixture(t, store.ReviewGrainFeature, "merge")
	f := load(t, root).Features["F-aaaaaa"]
	f.Gates["evals"] = store.Gate{Status: store.GateStatusFail}
	transferWrite(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml", f)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, []string{"evals"}, res.Crossings[2].PolicyViolations)
	require.Len(t, res.Crossings[2].HistoricalPolicyViolations, 1)
	require.Equal(t, store.GateStatusPending, res.Crossings[2].HistoricalPolicyViolations[0].Status)
	violations := load(t, root).Features["F-aaaaaa"].PolicyViolations
	require.Len(t, violations, 2)
	require.Empty(t, violations[0].SourceRevision.Value)
	require.Equal(t, store.GateStatusFail, violations[0].Status)
	require.NotEmpty(t, violations[1].SourceRevision.Value)
	require.Equal(t, store.GateStatusPending, violations[1].Status)
}

func TestRecoveredReviewReopenClearsWitnessAndIsolatesNewWork(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root, record := recoveredSyncFixture(t, grain, "merge")
			_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			originalEvents := recoveryEvents(t, root, grain)
			require.Len(t, originalEvents, 1)
			deps := seqDeps()
			rgAddTask(t, root, deps, "form-engine", "certification-form-loading", "later-work")
			driveWith(t, root, deps, store.NounTask, "form-engine/certification-form-loading/later-work", mutate.TransitionInput{Verb: "approve"})
			before := snapshotDir(t, root)
			err = tryTransition(t, root, deps, store.NounTask, "form-engine/certification-form-loading/later-work", mutate.TransitionInput{Verb: "start"})
			var blocked *mutate.BranchSafetyError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, "aggregate_round_closed", blocked.Reason)
			requireNoWrites(t, before, snapshotDir(t, root))
			res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Len(t, res.Reopened, 2)
			tree := load(t, root)
			require.Nil(t, tree.Features["F-aaaaaa"].RecoveredReview)
			require.Nil(t, tree.Epics["E-aaaaaa"].RecoveredReview)
			started := driveWith(t, root, deps, store.NounTask, "form-engine/certification-form-loading/later-work", mutate.TransitionInput{Verb: "start"})
			require.NotEqual(t, record.Branch, started.AggregateBranch)
			require.Equal(t, record.Head.Value[4:], gitOutM(t, root, "rev-parse", "refs/heads/"+record.Branch), "starting new work never resets the old reviewed ref")
			require.Equal(t, originalEvents, recoveryEvents(t, root, grain), "reopening preserves the old round's complete recovery audit")
			// Even a later submission on old ancestry cannot borrow its witness.
			task := branchTask(t, root, "form-engine/certification-form-loading/later-work")
			stageSyncReview(t, root, task.ID, syncSecondTaskBranch)
			res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Empty(t, res.Crossings)
			require.True(t, res.Awaiting[0].ReviewMissing)
		})
	}
}

func recoveryEvents(t *testing.T, root, grain string) []events.Event {
	t.Helper()
	all, err := events.List(load(t, root))
	require.NoError(t, err)
	var recovered []events.Event
	for _, event := range all {
		if event.Type == "recover_review."+grain {
			recovered = append(recovered, event)
		}
	}
	return recovered
}

func TestRecoveredSyncPersistsFeaturePolicyBeforeDelayedRollupWithoutDuplicates(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root, _ := recoveredSyncFixture(t, grain, "merge")
			f := load(t, root).Features["F-aaaaaa"]
			delete(f.Gates, "tests")
			transferWrite(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml", f)
			res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Equal(t, []string{"T-aaaaaa", "T-aaaaab"}, crossedIDs(res))
			f = load(t, root).Features["F-aaaaaa"]
			require.Equal(t, "active", f.Status)
			require.Len(t, f.PolicyViolations, 1, "historical feature policy is visible as soon as its riders cross")
			require.Equal(t, []string{"C8"}, f.PolicyViolations[0].UnresolvedCriteria)
			original := f.PolicyViolations[0]
			before := snapshotDir(t, root)
			again, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Empty(t, again.Crossings)
			requireNoWrites(t, before, snapshotDir(t, root))
			f.Gates["tests"] = store.Gate{Status: store.GateStatusPass}
			transferWrite(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml", f)
			res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Equal(t, []string{"F-aaaaaa", "E-aaaaaa"}, crossedIDs(res))
			require.Len(t, res.Crossings[0].HistoricalPolicyViolations, 1)
			violations := load(t, root).Features["F-aaaaaa"].PolicyViolations
			require.Equal(t, []store.PolicyViolation{original}, violations, "delayed rollup retains the original crossing observation and dedups historical source/node/gate")
		})
	}
}

func TestRecoveredContainedAlternateIntegrationCannotBeAsserted(t *testing.T) {
	t.Parallel()
	root, _ := recoveredSyncFixture(t, store.ReviewGrainFeature, "contained")
	gitRunM(t, root, "branch", "release", "main")
	before := snapshotDir(t, root)
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{Into: "release", AcceptFastForward: []string{"F-aaaaaa"}})
	require.Error(t, err)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestRecoveredSyncRetainsUnknownHistoricalPolicy(t *testing.T) {
	t.Parallel()
	root, _ := recoveredSyncFixture(t, store.ReviewGrainFeature, "merge")
	f := load(t, root).Features["F-aaaaaa"]
	f.RecoveredReview.PolicySnapshots[1].Observations[0].Status = "unknown"
	transferWrite(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml", f)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Crossings[2].HistoricalPolicyViolations, 1)
	require.Equal(t, "unknown", res.Crossings[2].HistoricalPolicyViolations[0].Status)
	require.Equal(t, "unknown", load(t, root).Features["F-aaaaaa"].PolicyViolations[0].Status)
}

type recoveredSyncRace struct {
	*vcs.Git
	change func()
}

// CommitRef is sync's last read before its first write: it names the
// integration commit the run resolved, so a change here lands after observation.
func (r *recoveredSyncRace) CommitRef(commit string) (store.RevisionRef, error) {
	ref, err := r.Git.CommitRef(commit)
	if err == nil && r.change != nil {
		change := r.change
		r.change = nil
		change()
	}
	return ref, err
}

func TestRecoveredSyncGuardsChangesAfterObservation(t *testing.T) {
	t.Parallel()
	for _, race := range []string{"integration_result", "aggregate_provenance", "retained_task_tip"} {
		t.Run(race, func(t *testing.T) {
			root, record := recoveredSyncFixture(t, store.ReviewGrainFeature, "merge")
			var lateTip string
			if race == "retained_task_tip" {
				gitRunM(t, root, "checkout", syncSecondTaskBranch)
				commitFile(t, root, "unreviewed-tip.txt", "later work")
				lateTip = gitOutM(t, root, "rev-parse", "HEAD")
				gitRunM(t, root, "checkout", "main")
				task := load(t, root).Tasks["T-aaaaab"]
				gitRunM(t, root, "branch", "-f", syncSecondTaskBranch, task.ReviewRef.Head.Value[4:])
			}
			var afterExternalChange map[string]string
			rev := &recoveredSyncRace{Git: vcs.New(root)}
			rev.change = func() {
				switch race {
				case "integration_result":
					gitRunM(t, root, "update-ref", "refs/heads/main", gitOutM(t, root, "rev-parse", record.MergeCommit.Value[4:]+"^1"))
				case "aggregate_provenance":
					f := load(t, root).Features["F-aaaaaa"]
					f.RecoveredReview.Reason = "concurrently replaced source"
					transferWrite(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml", f)
				case "retained_task_tip":
					gitRunM(t, root, "update-ref", "refs/heads/"+syncSecondTaskBranch, lateTip)
				}
				afterExternalChange = snapshotDir(t, root)
			}
			_, err := mutate.Sync(load(t, root), syncDeps(), rev, mutate.SyncInput{})
			require.Error(t, err)
			require.NotNil(t, afterExternalChange)
			requireNoWrites(t, afterExternalChange, snapshotDir(t, root))
			require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
			require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaab"].Status)
		})
	}
}

func TestRecoveredEpicPolicyOnlyAttachesToParticipatingFeature(t *testing.T) {
	t.Parallel()
	root, _ := recoveredSyncFixture(t, store.ReviewGrainEpic, "merge")
	key, taskKey := "form-engine/independent-work", "form-engine/independent-work/already-shipped"
	feature := &store.Feature{ID: "F-aaaaab", Key: &key, Status: "active", ChangeType: "create", ReviewGrain: store.ReviewGrainTask,
		GatesBlock: store.GatesBlock{Gates: map[string]store.Gate{"build": {Status: store.GateStatusPass}, "tests": {Status: store.GateStatusPass}, "evals": {Status: store.GateStatusPass}}}}
	dir := ".tiller/epics/form-engine/features/independent-work"
	transferWrite(t, root, dir+"/feature.yml", feature)
	transferBytes(t, root, dir+"/feature.md", []byte("# Independently shipped work\n"))
	transferBytes(t, root, dir+"/feature.eval.md", []byte("---\nnode: F-aaaaab\ncriteria:\n  - id: C1\n    statement: Independent work is included\n    required: true\n    polarity: introduce\n    binding: {type: manual}\n---\n"))
	task := &store.Task{ID: "T-aaaaac", Key: &taskKey, Status: "done", Started: true, Owner: "jc", ProposedBy: "jc", DoneBasis: "ancestry"}
	transferWrite(t, root, dir+"/tasks/already-shipped/task.yml", task)
	commitPaths(t, root, "independently shipped feature", dir)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Contains(t, crossedIDs(res), "F-aaaaab")
	tree := load(t, root)
	require.Equal(t, "done", tree.Features["F-aaaaab"].Status)
	require.Empty(t, tree.Features["F-aaaaab"].PolicyViolations, "an independent review never inherits the epic recovery's historical gates")
	require.Len(t, tree.Features["F-aaaaaa"].PolicyViolations, 1)
}

func TestConsumedRecoveryDoesNotCrossLaterSubmissionBeforeReopen(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root, _ := recoveredSyncFixture(t, grain, "merge")
			_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			deps := seqDeps()
			rgAddTask(t, root, deps, "form-engine", "certification-form-loading", "premature-submission")
			driveWith(t, root, deps, store.NounTask, "form-engine/certification-form-loading/premature-submission", mutate.TransitionInput{Verb: "approve"})
			task := branchTask(t, root, "form-engine/certification-form-loading/premature-submission")
			stageSyncReview(t, root, task.ID, syncSecondTaskBranch)
			assertedID := "F-aaaaaa"
			if grain == store.ReviewGrainEpic {
				assertedID = "E-aaaaaa"
			}
			before := snapshotDir(t, root)
			_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{assertedID}})
			var refusal *mutate.AttestNotEligibleError
			require.ErrorAs(t, err, &refusal)
			requireNoWrites(t, before, snapshotDir(t, root))
			res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Empty(t, res.Crossings)
			require.Len(t, res.Reopened, 2)
			require.Len(t, res.Awaiting, 1)
			require.True(t, res.Awaiting[0].ReviewMissing)
			require.Equal(t, "in_review", load(t, root).Tasks[task.ID].Status, "the consumed witness never closes later work")
		})
	}
}

func TestRecoveredSyncUsesExactSquashBeyondOrdinaryScanWindow(t *testing.T) {
	t.Parallel()
	root, record := recoveredSyncFixture(t, store.ReviewGrainFeature, "squash")
	for range 55 {
		gitRunM(t, root, "commit", "--allow-empty", "-m", "later integration traffic")
	}
	gitRunM(t, root, "branch", "-D", record.Branch, syncFirstTaskBranch, syncSecondTaskBranch)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 4)
	for _, crossing := range res.Crossings[:2] {
		require.Equal(t, "content", crossing.Basis)
		require.Equal(t, record.MergeCommit.Value, crossing.MatchedCommit)
	}
}

func TestRecoveredSyncWhitespaceMismatchNeverFallsBackToAssertion(t *testing.T) {
	t.Parallel()
	root, record := recoveredSyncFixture(t, store.ReviewGrainFeature, "squash")
	gitRunM(t, root, "checkout", "-b", "incorrect-squash", record.MergeCommit.Value[4:]+"^1")
	transferBytes(t, root, "first.txt", []byte("  "+syncFirstTaskBranch+"\n"))
	transferBytes(t, root, "second.txt", []byte(syncSecondTaskBranch+"\n"))
	gitCommitAllM(t, root, "whitespace-altered whole squash patch")
	incorrect := gitOutM(t, root, "rev-parse", "HEAD")
	rev := vcs.New(root)
	base := gitOutM(t, root, "rev-parse", record.MergeCommit.Value[4:]+"^1")
	wantID, err := rev.PatchID(base, record.Head.Value[4:])
	require.NoError(t, err)
	wrongID, err := rev.PatchID(base, incorrect)
	require.NoError(t, err)
	require.NotEmpty(t, wantID)
	require.Equal(t, wantID, wrongID, "the fixture differs only in whitespace ignored by stable patch-ID")
	wantText, err := rev.PatchDigest(base, record.Head.Value[4:])
	require.NoError(t, err)
	wrongText, err := rev.PatchDigest(base, incorrect)
	require.NoError(t, err)
	require.NotEqual(t, wantText, wrongText)
	gitRunM(t, root, "checkout", "main")
	// Preserve the current records while replacing integration's Git result.
	gitRunM(t, root, "update-ref", "refs/heads/main", incorrect)
	f := load(t, root).Features["F-aaaaaa"]
	f.RecoveredReview.MergeCommit.Value = "git:" + incorrect
	transferWrite(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml", f)
	before := snapshotDir(t, root)
	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{f.ID}})
	var refusal *mutate.RecoverReviewError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, "result_patch_mismatch", refusal.Reason)
	require.Equal(t, f.ID, refusal.NodeID)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestRecoveredEpicGuardsParticipatingFeatureGrainAfterObservation(t *testing.T) {
	t.Parallel()
	root, _ := recoveredSyncFixture(t, store.ReviewGrainEpic, "merge")
	var afterExternalChange map[string]string
	rev := &recoveredSyncRace{Git: vcs.New(root)}
	rev.change = func() {
		feature := load(t, root).Features["F-aaaaaa"]
		feature.ReviewGrain = store.ReviewGrainTask
		transferWrite(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml", feature)
		afterExternalChange = snapshotDir(t, root)
	}
	_, err := mutate.Sync(load(t, root), syncDeps(), rev, mutate.SyncInput{})
	require.Error(t, err)
	require.NotNil(t, afterExternalChange)
	requireNoWrites(t, afterExternalChange, snapshotDir(t, root))
	tree := load(t, root)
	require.Equal(t, "in_review", tree.Tasks["T-aaaaaa"].Status)
	require.Equal(t, "in_review", tree.Tasks["T-aaaaab"].Status)
	require.Equal(t, store.ReviewGrainTask, tree.Features["F-aaaaaa"].ReviewGrain, "sync preserves the concurrent external change rather than overwriting it")
}

func splitRecoveredFixtureFeatures(t *testing.T, root string) {
	t.Helper()
	tree := load(t, root)
	source := tree.Features["F-aaaaaa"]
	second := *source
	key := "form-engine/second-feature"
	second.ID, second.Key, second.Order = "F-aaaaab", &key, nil
	dir := ".tiller/epics/form-engine/features/second-feature"
	transferWrite(t, root, dir+"/feature.yml", &second)
	for _, name := range []string{"feature.md", "feature.eval.md"} {
		content, err := store.ReadRepositoryFile(root, ".tiller/epics/form-engine/features/certification-form-loading/"+name)
		require.NoError(t, err)
		if name == "feature.eval.md" {
			content = []byte(strings.ReplaceAll(string(content), source.ID, second.ID))
		}
		transferBytes(t, root, dir+"/"+name, content)
	}
	task := tree.Tasks["T-aaaaab"]
	oldDir := filepath.Dir(taskYMLPath(t, root, task.ID))
	newDir := filepath.Join(root, dir, "tasks", "validate-the-schema")
	require.NoError(t, os.MkdirAll(filepath.Dir(newDir), 0o755))
	require.NoError(t, os.Rename(oldDir, newDir))
	taskKey := key + "/validate-the-schema"
	task.Key = &taskKey
	task.AcceptedAgainst.Spec.Path = dir + "/tasks/validate-the-schema/task.md"
	transferWrite(t, root, dir+"/tasks/validate-the-schema/task.yml", task)
}

func TestRecoveredEpicRefreshesParticipatingFeatureGuardsAcrossRollups(t *testing.T) {
	t.Parallel()
	root, _ := recoveredSyncFixture(t, store.ReviewGrainEpic, "merge", true)
	second := load(t, root).Features["F-aaaaab"]
	delete(second.Gates, "tests")
	transferWrite(t, root, ".tiller/epics/form-engine/features/second-feature/feature.yml", second)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, []string{"T-aaaaaa", "F-aaaaaa", "T-aaaaab"}, crossedIDs(res), "the second feature's task can cross after the first feature updated its guarded document")
	tree := load(t, root)
	require.Equal(t, "done", tree.Features["F-aaaaaa"].Status)
	require.Equal(t, "active", tree.Features["F-aaaaab"].Status)
	for _, feature := range tree.Features {
		require.Len(t, feature.PolicyViolations, 1)
	}
	original := tree.Features["F-aaaaab"].PolicyViolations[0]
	second = tree.Features["F-aaaaab"]
	second.Gates["tests"] = store.Gate{Status: store.GateStatusPass}
	transferWrite(t, root, ".tiller/epics/form-engine/features/second-feature/feature.yml", second)
	res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, []string{"F-aaaaab", "E-aaaaaa"}, crossedIDs(res))
	require.Equal(t, []store.PolicyViolation{original}, load(t, root).Features["F-aaaaab"].PolicyViolations, "delayed policy persistence and rollup refresh guards without duplicating history")
}
