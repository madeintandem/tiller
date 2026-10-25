package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func recoveryFixture(t *testing.T, grain string) (string, mutate.Deps, mutate.RecoverReviewInput) {
	return recoveryFixtureMode(t, grain, "merge")
}

func recoveryFixtureMode(t *testing.T, grain, mode string, extraTaskKeys ...string) (string, mutate.Deps, mutate.RecoverReviewInput) {
	t.Helper()
	d := seqDeps()
	taskKeys := append([]string{"alfa"}, extraTaskKeys...)
	root := rgLayFeature(t, d, "pay", "checkout", taskKeys...)
	noun, key := store.NounFeature, "pay/checkout"
	if grain == store.ReviewGrainEpic {
		noun, key = store.NounEpic, "pay"
	}
	_, err := mutate.Configure(load(t, root), d, mutate.ConfigureInput{Noun: noun, Key: key, ReviewGrain: grain})
	require.NoError(t, err)
	for i, taskKey := range taskKeys {
		rgDriveTaskToInReview(t, root, d, "pay", "checkout", taskKey, i+1)
	}
	branch := featureByKey(load(t, root), "pay/checkout").Branch
	if grain == store.ReviewGrainEpic {
		branch = resolveNode(t, load(t, root), store.NounEpic, "pay").(*store.Epic).Branch
	}
	var originalManifest []byte
	switch mode {
	case "missing_policy", "invalid_policy", "null_policy", "unknown_policy", "null_required_policy", "empty_policy", "pending_task_c8", "invalid_task_criterion":
		var err error
		originalManifest, err = os.ReadFile(filepath.Join(root, ".tiller/tiller.yml"))
		require.NoError(t, err)
		var policy map[string]any
		require.NoError(t, yaml.Unmarshal(originalManifest, &policy))
		sections := policy["gates"].(map[string]any)
		switch mode {
		case "missing_policy":
			delete(policy, "gates")
		case "invalid_policy":
			sections["required"].(map[string]any)["build"].(map[string]any)["kind"] = "invented"
		case "null_policy":
			policy["gates"] = nil
		case "unknown_policy":
			sections["required_typo"] = sections["required"]
			delete(sections, "required")
		case "null_required_policy":
			sections["required"] = nil
		case "empty_policy":
			policy["gates"] = map[string]any{}
		default:
			sections["required"].(map[string]any)["evals"].(map[string]any)["on"] = []string{store.NounTask}
		}
		historicalPolicy, err := yaml.Marshal(policy)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/tiller.yml"), historicalPolicy, 0600))
	}
	if mode == "pending_c8" || mode == "pending_task_c8" || mode == "invalid_task_criterion" {
		feature := featureByKey(load(t, root), "pay/checkout")
		contract := strings.Replace(greenEvalMD(feature.ID), "---\n\n# Acceptance", "  - id: C8\n    statement: Human attestation\n    polarity: introduce\n    binding: { type: manual }\n    required: true\n---\n\n# Acceptance", 1)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/feature.eval.md"), []byte(contract), 0600))
		feature.Gates = map[string]store.Gate{"evals": {Status: store.GateStatusPending, Criteria: map[string]store.Criterion{"C1": {Status: store.CriterionStatusPass}, "C8": {Status: store.CriterionStatusNeedsAttestation, Evidence: []store.Evidence{{Kind: "review", Ref: "historical-attestation-request"}}}}}}
		data, err := yaml.Marshal(feature)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/feature.yml"), data, 0600))
		if mode != "pending_c8" {
			task := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
			criterionStatus := store.CriterionStatusNeedsAttestation
			if mode == "invalid_task_criterion" {
				criterionStatus = "unrecognized"
			}
			task.Gates = map[string]store.Gate{"evals": {Status: store.GateStatusPending, Criteria: map[string]store.Criterion{"C1": {Status: store.CriterionStatusPass}, "C8": {Status: criterionStatus, Evidence: []store.Evidence{{Kind: "review", Ref: "historical-task-attestation-request"}}}}}}
			taskData, err := yaml.Marshal(task)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/tasks/alfa/task.yml"), taskData, 0600))
		}
	}
	if mode == "sha256" {
		for _, task := range load(t, root).Tasks {
			specPath := filepath.Join(".tiller", "epics", "pay", "features", "checkout", "tasks", filepath.Base(*task.Key), "task.md")
			blob, err := vcs.New(root).BlobHash(specPath)
			require.NoError(t, err)
			// Historical approved state names the actual object in this repository's
			// format, independently of legacy raw-SHA1 basis recording.
			task.AcceptedAgainst.Spec.Value = "git:" + blob
			data, err := yaml.Marshal(task)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(filepath.Join(root, specPath)), "task.yml"), data, 0600))
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "implementation.txt"), []byte("alpha\n"), 0600))
	if mode == "schema9" {
		data, err := os.ReadFile(filepath.Join(root, ".tiller/tiller.yml"))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/tiller.yml"), []byte(strings.Replace(string(data), "schema_version: 12", "schema_version: 9", 1)), 0600))
	}
	gitCommitAllM(t, root, "complete reviewed implementation")
	head, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	gitRunM(t, root, "checkout", "main")
	switch mode {
	case "squash", "squash_whitespace", "squash_partial":
		gitRunM(t, root, "merge", "--squash", branch)
		if mode == "squash_whitespace" {
			require.NoError(t, os.WriteFile(filepath.Join(root, "implementation.txt"), []byte("alpha \n"), 0600))
		}
		if mode == "squash_partial" {
			require.NoError(t, os.WriteFile(filepath.Join(root, "implementation.txt"), []byte("different\n"), 0600))
		}
		gitCommitAllM(t, root, "human squash")
	case "contained":
		gitRunM(t, root, "merge", "--ff-only", branch)
	default:
		gitRunM(t, root, "merge", "--no-ff", "-m", "human merge", branch)
	}
	result, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	if originalManifest != nil {
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/tiller.yml"), originalManifest, 0600))
		gitCommitAllM(t, root, "restore current valid policy independently")
	}
	if mode == "pending_c8" || mode == "pending_task_c8" || mode == "invalid_task_criterion" {
		feature := featureByKey(load(t, root), "pay/checkout")
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/feature.eval.md"), []byte(greenEvalMD(feature.ID)), 0600))
		feature.Gates = map[string]store.Gate{"build": {Status: store.GateStatusPass}, "tests": {Status: store.GateStatusPass}, "evals": {Status: store.GateStatusPass, Criteria: map[string]store.Criterion{"C1": {Status: store.CriterionStatusPass}}}}
		data, err := yaml.Marshal(feature)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/feature.yml"), data, 0600))
		if mode != "pending_c8" {
			task := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
			task.Gates = map[string]store.Gate{"build": {Status: store.GateStatusPass}, "tests": {Status: store.GateStatusPass}, "evals": {Status: store.GateStatusPass, Criteria: map[string]store.Criterion{"C1": {Status: store.CriterionStatusPass}}}}
			taskData, err := yaml.Marshal(task)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/tasks/alfa/task.yml"), taskData, 0600))
		}
		gitCommitAllM(t, root, "approved current contract omits historical C8")
	}
	if mode == "schema9" {
		data, err := os.ReadFile(filepath.Join(root, ".tiller/tiller.yml"))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/tiller.yml"), []byte(strings.Replace(string(data), "schema_version: 9", "schema_version: 12", 1)), 0600))
		gitCommitAllM(t, root, "adopt additive schema ten")
	}
	return root, d, mutate.RecoverReviewInput{Noun: noun, Key: key, Head: strings.TrimPrefix(head.Value, "git:"), MergeCommit: strings.TrimPrefix(result.Value, "git:"), Reason: "Authorized historical recovery"}
}

func TestRecoverReviewRecordOnlyDeletedRefsAndTerminalRetry(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root, d, in := recoveryFixture(t, grain)
			before := load(t, root)
			task := resolveNode(t, before, store.NounTask, "pay/checkout/alfa").(*store.Task)
			branch := featureByKey(before, "pay/checkout").Branch
			if grain == store.ReviewGrainEpic {
				branch = resolveNode(t, before, store.NounEpic, "pay").(*store.Epic).Branch
			}
			gitRunM(t, root, "branch", "-D", branch)
			gitRunM(t, root, "branch", "-D", task.Branch)
			r, err := mutate.RecoverReview(before, d, in)
			require.NoError(t, err)
			require.True(t, r.Changed)
			require.Equal(t, "ancestry", r.RecoveredReview.Basis)
			require.Equal(t, "active", featureByKey(load(t, root), "pay/checkout").Status)
			require.Equal(t, "in_review", load(t, root).Tasks[task.ID].Status)
			require.Equal(t, task.Owner, load(t, root).Tasks[task.ID].Owner)
			require.NotEmpty(t, r.RecoveredReview.PolicySnapshots)
			gitCommitAllM(t, root, "record recovery")
			_, err = mutate.Sync(load(t, root), d, vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Equal(t, "done", load(t, root).Tasks[task.ID].Status)
			evs, err := events.List(load(t, root))
			require.NoError(t, err)
			snapshot := snapshotDir(t, filepath.Join(root, ".tiller"))
			in.Reason = "Different explanation must not rewrite original"
			retry, err := mutate.RecoverReview(load(t, root), d, in)
			require.NoError(t, err)
			require.False(t, retry.Changed)
			require.Equal(t, r.RecoveredReview.Reason, retry.RecoveredReview.Reason)
			requireNoWrites(t, snapshot, snapshotDir(t, filepath.Join(root, ".tiller")))
			after, err := events.List(load(t, root))
			require.NoError(t, err)
			require.Len(t, after, len(evs))
		})
	}
}

func TestRecoverReviewRefusalsWriteNothing(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"unrelated_result", "future_result", "task_tip_advanced", "changed_scope", "changed_owner", "changed_branch", "task_owned_pr", "aggregate_branch_changed", "wrong_grain", "terminal", "unknown_actor", "new_unsubmitted_rider", "remote_without_pr", "nonpositive_pr", "missing_object", "dirty_prose", "staged_prose", "wrong_checkout", "changed_identity"} {
		t.Run(kind, func(t *testing.T) {
			root, d, in := recoveryFixture(t, store.ReviewGrainFeature)
			task := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
			switch kind {
			case "unrelated_result":
				parent, err := vcs.New(root).CommitParents(in.MergeCommit)
				require.NoError(t, err)
				in.MergeCommit = parent[0]
			case "future_result":
				gitRunM(t, root, "checkout", "-b", "future-result")
				gitRunM(t, root, "commit", "--allow-empty", "-m", "future result not on integration")
				result, err := vcs.New(root).CurrentCommit()
				require.NoError(t, err)
				in.MergeCommit = strings.TrimPrefix(result.Value, "git:")
				gitRunM(t, root, "checkout", "main")
			case "task_tip_advanced":
				gitRunM(t, root, "checkout", task.Branch)
				require.NoError(t, os.WriteFile(filepath.Join(root, "new-work"), []byte("later work"), 0600))
				gitCommitAllM(t, root, "later task work")
				gitRunM(t, root, "checkout", "main")
			case "changed_scope":
				task.AcceptedAgainst.Spec.Value = "git:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				data, err := yaml.Marshal(task)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/tasks/alfa/task.yml"), data, 0600))
				gitCommitAllM(t, root, "different approved task scope")
			case "remote_without_pr":
				gitRunM(t, root, "remote", "add", "origin", "https://example.invalid/repo.git")
			case "dirty_prose":
				require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/feature.md"), []byte("uncommitted contract"), 0600))
			case "staged_prose":
				path := filepath.Join(root, ".tiller/epics/pay/features/checkout/feature.md")
				original, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, append(append([]byte(nil), original...), []byte("\nStaged contract edit\n")...), 0600))
				gitRunM(t, root, "add", path)
				require.NoError(t, os.WriteFile(path, original, 0600))
			case "wrong_checkout":
				gitRunM(t, root, "checkout", "-b", "scratch")
			case "changed_identity":
				r, err := mutate.RecoverReview(load(t, root), d, in)
				require.NoError(t, err)
				require.True(t, r.Changed)
				in.PR = intPtr(42)
			case "changed_owner", "changed_branch", "task_owned_pr":
				if kind == "changed_owner" {
					task.Owner = "another"
				}
				if kind == "changed_branch" {
					task.Branch = "different-task-round"
				}
				if kind == "task_owned_pr" {
					task.PR = intPtr(42)
				}
				data, err := yaml.Marshal(task)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/tasks/alfa/task.yml"), data, 0600))
				gitCommitAllM(t, root, "different current task identity")
			case "new_unsubmitted_rider":
				rgAddTask(t, root, d, "pay", "checkout", "new-rider")
				gitCommitAllM(t, root, "current rider omitted by historical head")
			case "nonpositive_pr":
				in.PR = intPtr(0)
			case "missing_object":
				in.Head = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			case "aggregate_branch_changed", "wrong_grain", "terminal":
				feature := featureByKey(load(t, root), "pay/checkout")
				if kind == "aggregate_branch_changed" {
					feature.Branch = "different-aggregate-round"
				}
				if kind == "wrong_grain" {
					feature.ReviewGrain = store.ReviewGrainTask
				}
				if kind == "terminal" {
					feature.Status = "done"
				}
				data, err := yaml.Marshal(feature)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/feature.yml"), data, 0600))
				gitCommitAllM(t, root, "different aggregate identity or lifecycle")
			case "unknown_actor":
				d.Contributor = "unregistered"
			}
			before := snapshotDir(t, filepath.Join(root, ".tiller"))
			_, err := mutate.RecoverReview(load(t, root), d, in)
			require.Error(t, err)
			requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
		})
	}
}

func TestRecoverReviewExactSquashBeyondScanAndContainedAssertion(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		for _, mode := range []string{"squash", "contained"} {
			t.Run(grain+"/"+mode, func(t *testing.T) {
				root, d, in := recoveryFixtureMode(t, grain, mode)
				if mode == "squash" {
					for range 55 {
						gitRunM(t, root, "commit", "--allow-empty", "-m", "later integration history")
					}
				}
				r, err := mutate.RecoverReview(load(t, root), d, in)
				require.NoError(t, err)
				expected := "content"
				if mode == "contained" {
					expected = "contained"
				}
				require.Equal(t, expected, r.RecoveredReview.Basis)
				gitCommitAllM(t, root, "record historical review")
				_, err = mutate.Sync(load(t, root), d, vcs.New(root), mutate.SyncInput{})
				require.NoError(t, err)
				task := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
				if mode == "contained" {
					require.Equal(t, "in_review", task.Status)
					_, err = mutate.Sync(load(t, root), d, vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{r.ID}})
					require.NoError(t, err)
					task = load(t, root).Tasks[task.ID]
					require.Equal(t, "human_attested", task.DoneBasis)
				} else {
					require.Equal(t, "content", task.DoneBasis)
				}
				require.Equal(t, "done", task.Status)
			})
		}
	}
}

func TestRecoverReviewIgnoresAdvancedAggregateAndCurrentFeatureContract(t *testing.T) {
	t.Parallel()
	root, d, in := recoveryFixture(t, store.ReviewGrainFeature)
	feature := featureByKey(load(t, root), "pay/checkout")
	branch := feature.Branch
	gitRunM(t, root, "checkout", branch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "old-branch-note"), []byte("later aggregate work"), 0600))
	gitCommitAllM(t, root, "advance old aggregate after merge")
	gitRunM(t, root, "checkout", "main")
	feature.AcceptedAgainst.Spec.Value = "git:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	feature.Gates = map[string]store.Gate{"evals": {Status: store.GateStatusPass}}
	data, err := yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/feature.yml"), data, 0600))
	gitCommitAllM(t, root, "current separately approved feature contract")
	before := featureByKey(load(t, root), "pay/checkout")
	r, err := mutate.RecoverReview(load(t, root), d, in)
	require.NoError(t, err)
	after := featureByKey(load(t, root), "pay/checkout")
	require.Equal(t, before.AcceptedAgainst, after.AcceptedAgainst)
	require.Equal(t, before.Gates, after.Gates)
	var pending bool
	for _, snapshot := range r.RecoveredReview.PolicySnapshots {
		for _, observation := range snapshot.Observations {
			if snapshot.NodeID == feature.ID && observation.Gate == "evals" && observation.Status == "pending" {
				pending = true
			}
		}
	}
	require.True(t, pending, "historical pending feature gates are independent of current pass")
}

func TestRecoverReviewStaleRefsAndSourcesLeaveNoRecoveryWrites(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"source", "integration_ref", "task_ref"} {
		t.Run(kind, func(t *testing.T) {
			root, d, in := recoveryFixture(t, store.ReviewGrainFeature)
			task := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
			var before map[string]string
			once := false
			d.Clock = renameCallbackClock{run: func() {
				if once {
					return
				}
				once = true
				switch kind {
				case "source":
					data, err := os.ReadFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/tasks/alfa/task.yml"))
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/pay/features/checkout/tasks/alfa/task.yml"), append(data, []byte("\n# concurrent source change\n")...), 0600))
				case "integration_ref":
					gitRunM(t, root, "commit", "--allow-empty", "-m", "concurrent integration change")
				case "task_ref":
					gitRunM(t, root, "branch", "-f", task.Branch, "main")
				}
				before = snapshotDir(t, filepath.Join(root, ".tiller"))
			}}
			_, err := mutate.RecoverReview(load(t, root), d, in)
			require.Error(t, err)
			require.True(t, once)
			requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
		})
	}
}

func TestRecoverReviewRefusesSubmissionStateOutsideReviewedTree(t *testing.T) {
	t.Parallel()
	root, d, in := recoveryFixture(t, store.ReviewGrainFeature)
	task := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
	in.Head = strings.TrimPrefix(task.ReviewRef.Head.Value, "git:")
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	_, err := mutate.RecoverReview(load(t, root), d, in)
	var refusal *mutate.RecoverReviewError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, task.ID, refusal.TaskID)
	requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
}

func TestRecoverReviewRejectsWhitespaceAndPartialSquash(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"squash_whitespace", "squash_partial"} {
		t.Run(mode, func(t *testing.T) {
			root, d, in := recoveryFixtureMode(t, store.ReviewGrainFeature, mode)
			before := snapshotDir(t, filepath.Join(root, ".tiller"))
			_, err := mutate.RecoverReview(load(t, root), d, in)
			var refusal *mutate.RecoverReviewError
			require.ErrorAs(t, err, &refusal)
			require.Equal(t, "result_patch_mismatch", refusal.Reason)
			requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
		})
	}
}

func TestRecoverReviewReadsHistoricalSchemaNineAfterAdditiveAdoption(t *testing.T) {
	t.Parallel()
	root, d, in := recoveryFixtureMode(t, store.ReviewGrainFeature, "schema9")
	r, err := mutate.RecoverReview(load(t, root), d, in)
	require.NoError(t, err)
	require.Equal(t, "ancestry", r.RecoveredReview.Basis)
	require.Equal(t, store.CurrentSchemaVersion, load(t, root).Manifest.SchemaVersion)
}

func TestRecoverReviewRefusesIndependentEpicOverride(t *testing.T) {
	t.Parallel()
	root, d, in := recoveryFixture(t, store.ReviewGrainEpic)
	id := rgAddFeature(t, root, d, "pay", "independent")
	_, err := mutate.Configure(load(t, root), d, mutate.ConfigureInput{Noun: store.NounFeature, Key: id, ReviewGrain: store.ReviewGrainTask})
	require.NoError(t, err)
	gitCommitAllM(t, root, "independent review outside recovered epic")
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	_, err = mutate.RecoverReview(load(t, root), d, in)
	var refusal *mutate.RecoverReviewError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, "independent_review_open", refusal.Reason)
	requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
}

func TestRecoverReviewRejectsUnrelatedLaterMergeWitness(t *testing.T) {
	t.Parallel()
	root, d, in := recoveryFixture(t, store.ReviewGrainFeature)
	gitRunM(t, root, "checkout", "-b", "unrelated")
	require.NoError(t, os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("unrelated change"), 0600))
	gitCommitAllM(t, root, "unrelated side")
	gitRunM(t, root, "checkout", "main")
	gitRunM(t, root, "commit", "--allow-empty", "-m", "later integration")
	gitRunM(t, root, "merge", "--no-ff", "-m", "unrelated later merge", "unrelated")
	result, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	in.MergeCommit = strings.TrimPrefix(result.Value, "git:")
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	_, err = mutate.RecoverReview(load(t, root), d, in)
	var refusal *mutate.RecoverReviewError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, "merge_witness_missing", refusal.Reason)
	requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
}

func TestRecoverReviewJournalEventCollisionCannotWriteOwner(t *testing.T) {
	t.Parallel()
	root, d, in := recoveryFixture(t, store.ReviewGrainFeature)
	existing, err := events.List(load(t, root))
	require.NoError(t, err)
	require.NotEmpty(t, existing)
	d.Mint = func(string) string { return existing[0].ID }
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	_, err = mutate.RecoverReview(load(t, root), d, in)
	require.Error(t, err)
	requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
	require.Nil(t, featureByKey(load(t, root), "pay/checkout").RecoveredReview)
}

func TestRecoverReviewHistoricalPolicyAbsenceNeverMeansAllClear(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"missing_policy", "invalid_policy", "null_policy", "unknown_policy", "null_required_policy"} {
		t.Run(mode, func(t *testing.T) {
			root, d, in := recoveryFixtureMode(t, store.ReviewGrainFeature, mode)
			before := snapshotDir(t, filepath.Join(root, ".tiller"))
			_, err := mutate.RecoverReview(load(t, root), d, in)
			var refusal *mutate.RecoverReviewError
			require.ErrorAs(t, err, &refusal)
			require.Contains(t, refusal.Reason, "historical_policy")
			requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
		})
	}
}

func TestRecoverReviewRetainsHistoricalC8AlongsideCurrentPass(t *testing.T) {
	t.Parallel()
	root, d, in := recoveryFixtureMode(t, store.ReviewGrainFeature, "pending_c8")
	current := featureByKey(load(t, root), "pay/checkout")
	r, err := mutate.RecoverReview(load(t, root), d, in)
	require.NoError(t, err)
	var observations []store.HistoricalPolicyObservation
	for _, snapshot := range r.RecoveredReview.PolicySnapshots {
		if snapshot.NodeID == current.ID {
			observations = append(observations, snapshot.Observations...)
		}
	}
	var pending *store.HistoricalPolicyObservation
	for i := range observations {
		if observations[i].Gate == "evals" {
			pending = &observations[i]
		}
	}
	require.NotNil(t, pending)
	require.Equal(t, "pending", pending.Status)
	require.Equal(t, []string{"C8"}, pending.UnresolvedCriteria)
	require.Contains(t, pending.EvidenceRefs, "historical-attestation-request")
	gitCommitAllM(t, root, "record recovery with historical pending C8")
	_, err = mutate.Sync(load(t, root), d, vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	after := featureByKey(load(t, root), "pay/checkout")
	require.Equal(t, current.AcceptedAgainst, after.AcceptedAgainst)
	require.Equal(t, current.Gates, after.Gates)
	var historicalWarning bool
	for _, warning := range after.PolicyViolations {
		if warning.Gate == "evals" && warning.SourceRevision.Value != "" {
			require.Equal(t, []string{"C8"}, warning.UnresolvedCriteria)
			historicalWarning = true
		}
	}
	require.True(t, historicalWarning)
}

func TestOrdinarySubmitCannotReplaceRecoveredBinding(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root, d, in := recoveryFixture(t, grain)
			recovered, err := mutate.RecoverReview(load(t, root), d, in)
			require.NoError(t, err)
			before := snapshotDir(t, filepath.Join(root, ".tiller"))
			_, err = mutate.SubmitNode(load(t, root), d, mutate.SubmitNodeInput{Noun: in.Noun, Key: in.Key, ReviewRef: &store.ReviewRef{Branch: recovered.RecoveredReview.Branch, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}})
			var refusal *mutate.RecoverReviewError
			require.ErrorAs(t, err, &refusal)
			require.Equal(t, "recovery_already_recorded", refusal.Reason)
			requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
		})
	}
}

func TestRecoverReviewConsolidatesSharedLineageAndProseGuards(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root, d, in := recoveryFixtureMode(t, grain, "merge", "bravo")
			require.NoError(t, os.WriteFile(filepath.Join(root, "unrelated-working-change.txt"), []byte("unrelated uncommitted work must remain untouched"), 0600))
			before := snapshotDir(t, filepath.Join(root, ".tiller"))
			recovered, err := mutate.RecoverReview(load(t, root), d, in)
			require.NoError(t, err)
			require.True(t, recovered.Changed)
			require.Len(t, recovered.RecoveredReview.Tasks, 2)
			after := snapshotDir(t, filepath.Join(root, ".tiller"))
			ownerPath := filepath.Join("epics", "pay", "features", "checkout", "feature.yml")
			if grain == store.ReviewGrainEpic {
				ownerPath = filepath.Join("epics", "pay", "epic.yml")
			}
			require.NotEqual(t, before[ownerPath], after[ownerPath])
			require.Len(t, after, len(before)+1, "only one distinct recovery event is added")
			for path, fingerprint := range before {
				if path != ownerPath && path != filepath.Join(".cache", "generation") {
					require.Equal(t, fingerprint, after[path], path)
				}
			}
			for _, task := range load(t, root).Tasks {
				require.Equal(t, "in_review", task.Status)
			}
		})
	}
}

func TestRecoverReviewExplicitEmptyHistoricalPolicyIsValid(t *testing.T) {
	t.Parallel()
	root, d, in := recoveryFixtureMode(t, store.ReviewGrainFeature, "empty_policy")
	recovered, err := mutate.RecoverReview(load(t, root), d, in)
	require.NoError(t, err)
	require.True(t, recovered.Changed)
	for _, snapshot := range recovered.RecoveredReview.PolicySnapshots {
		require.Empty(t, snapshot.Observations)
	}
}

func TestRecoverReviewTaskScopedHistoricalEvalsRetainC8(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root, d, in := recoveryFixtureMode(t, grain, "pending_task_c8")
			current := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
			recovered, err := mutate.RecoverReview(load(t, root), d, in)
			require.NoError(t, err)
			var taskObservation *store.HistoricalPolicyObservation
			for _, snapshot := range recovered.RecoveredReview.PolicySnapshots {
				for i := range snapshot.Observations {
					observation := &snapshot.Observations[i]
					if snapshot.NodeID == current.FeatureID {
						require.NotEqual(t, "evals", observation.Gate, "task-only policy must not invent a feature evals requirement")
					}
					if snapshot.NodeID == current.ID && observation.Gate == "evals" {
						taskObservation = observation
					}
				}
			}
			require.NotNil(t, taskObservation)
			require.Equal(t, "pending", taskObservation.Status)
			require.Equal(t, []string{"C8"}, taskObservation.UnresolvedCriteria)
			require.Equal(t, []string{"historical-task-attestation-request"}, taskObservation.EvidenceRefs)
			gitCommitAllM(t, root, "record task-scoped historical C8 recovery")
			_, err = mutate.Sync(load(t, root), d, vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			after := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
			require.Equal(t, "done", after.Status)
			require.Equal(t, current.Gates, after.Gates)
			var historicalWarning bool
			for _, warning := range after.PolicyViolations {
				if warning.Gate == "evals" && warning.SourceRevision.Value != "" {
					require.Equal(t, []string{"C8"}, warning.UnresolvedCriteria)
					require.Equal(t, []string{"historical-task-attestation-request"}, warning.EvidenceRefs)
					historicalWarning = true
				}
			}
			require.True(t, historicalWarning)
		})
	}
}

func TestRecoverReviewInvalidHistoricalTaskCriterionRefusesWithoutWrites(t *testing.T) {
	t.Parallel()
	root, d, in := recoveryFixtureMode(t, store.ReviewGrainFeature, "invalid_task_criterion")
	task := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	_, err := mutate.RecoverReview(load(t, root), d, in)
	var refusal *mutate.RecoverReviewError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, "historical_policy_invalid", refusal.Reason)
	require.Equal(t, task.ID, refusal.TaskID)
	requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
}

func TestRecoverReviewGitCleanNormalizedRelevantInputs(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"crlf", "clean_filter"} {
		t.Run(mode, func(t *testing.T) {
			root, d, in := recoveryFixture(t, store.ReviewGrainFeature)
			attributes := ".tiller/** text eol=crlf\n"
			if mode == "crlf" {
				gitRunM(t, root, "config", "core.autocrlf", "true")
			} else {
				gitRunM(t, root, "config", "core.autocrlf", "false")
				gitRunM(t, root, "config", "filter.recovery-clean.clean", "tr -d '\\015'")
				gitRunM(t, root, "config", "filter.recovery-clean.smudge", "cat")
				attributes = ".tiller/** -text filter=recovery-clean\n"
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, ".gitattributes"), []byte(attributes), 0600))
			gitCommitAllM(t, root, "configure Git normalization for relevant source")
			require.NoError(t, filepath.WalkDir(filepath.Join(root, ".tiller"), func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				extension := filepath.Ext(path)
				if extension != ".md" && extension != ".yml" {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				normalized := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n", "\r\n")
				return os.WriteFile(path, []byte(normalized), 0600)
			}))
			// Refresh the index's conversion/stat metadata after changing attributes
			// and rewriting the checkout. The normalized objects must remain equal
			// to HEAD; the following status assertion rejects any staged difference.
			gitRunM(t, root, "add", "--renormalize", ".tiller")
			dirty, err := vcs.New(root).DirtyUnder(":(literal).tiller")
			require.NoError(t, err)
			require.Empty(t, dirty, "fixture must be clean under Git normalization")
			current := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
			recovered, err := mutate.RecoverReview(load(t, root), d, in)
			require.NoError(t, err)
			require.True(t, recovered.Changed)
			after := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
			require.Equal(t, current.Gates, after.Gates)
			require.Equal(t, current.AcceptedAgainst, after.AcceptedAgainst)
			require.Equal(t, current.Owner, after.Owner)
			require.Equal(t, "in_review", after.Status)
		})
	}
}

// Serial: t.Setenv sets GIT_DEFAULT_HASH for every git the process spawns.
func TestRecoverReviewSHA256RepositoryUsesGitObjectIdentity(t *testing.T) {
	t.Setenv("GIT_DEFAULT_HASH", "sha256")
	root, d, in := recoveryFixtureMode(t, store.ReviewGrainFeature, "sha256")
	recovered, err := mutate.RecoverReview(load(t, root), d, in)
	require.NoError(t, err)
	require.Len(t, strings.TrimPrefix(recovered.RecoveredReview.Head.Value, "git:"), 64)
	for _, snapshot := range recovered.RecoveredReview.PolicySnapshots {
		require.Len(t, strings.TrimPrefix(snapshot.Source.Value, "git:"), 64)
	}
	require.Equal(t, "in_review", resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task).Status)
}

func TestRecoverReviewRefusesRevisionExpressionsBeforeReadingGit(t *testing.T) {
	t.Parallel()
	root, d, in := recoveryFixture(t, store.ReviewGrainFeature)
	for _, bad := range []string{"main", "HEAD^", in.Head + "^{commit}", in.Head[:3]} {
		attempt := in
		attempt.Head = bad
		before := snapshotDir(t, root)
		_, err := mutate.RecoverReview(load(t, root), d, attempt)
		require.ErrorContains(t, err, "must be hexadecimal commit IDs", bad)
		requireNoWrites(t, before, snapshotDir(t, root))
	}
}
