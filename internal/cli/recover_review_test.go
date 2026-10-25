package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRecoveryFlagsRejectExplicitPresenceAcrossNouns(t *testing.T) {
	layReadTree(t)
	for _, noun := range []string{"task", "feature", "epic", "bug", "chore", "tweak"} {
		for _, flag := range []string{"--recover-merged=false", "--head=", "--merge-commit="} {
			t.Run(noun+flag, func(t *testing.T) {
				out, _, code := execCLI(t, "--json", "submit", noun, "not-a-node", flag)
				require.Equal(t, 2, code)
				var envelope struct {
					OK    bool      `json:"ok"`
					Error ErrorBody `json:"error"`
				}
				require.NoError(t, json.Unmarshal([]byte(out), &envelope))
				require.False(t, envelope.OK)
				require.Equal(t, "recovery_flags_misuse", envelope.Error.Reason)
			})
		}
		if noun != "feature" && noun != "epic" {
			t.Run(noun+"enabled", func(t *testing.T) {
				out, _, code := execCLI(t, "--json", "submit", noun, "not-a-node", "--recover-merged", "--head=abc", "--merge-commit=def", "--reason=authorized")
				require.Equal(t, 2, code)
				var envelope struct {
					Error ErrorBody `json:"error"`
				}
				require.NoError(t, json.Unmarshal([]byte(out), &envelope))
				require.Equal(t, "recovery_flags_misuse", envelope.Error.Reason)
			})
		}
	}
	for _, args := range [][]string{
		{"--recover-merged", "--head=", "--merge-commit=def", "--reason=authorized"},
		{"--recover-merged", "--head=abc", "--merge-commit=", "--reason=authorized"},
		{"--recover-merged", "--head=abc", "--merge-commit=def", "--reason="},
		{"--recover-merged", "--head=abc", "--merge-commit=def", "--reason=authorized", "--assume-question="},
	} {
		out, _, code := execCLI(t, append([]string{"--json", "submit", "feature", "not-a-node"}, args...)...)
		require.Equal(t, 2, code)
		var envelope struct {
			OK bool `json:"ok"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &envelope))
		require.False(t, envelope.OK)
	}
}

func TestRecoveryRefusalEnvelopeRetainsNamedPrerequisite(t *testing.T) {
	failure := &mutate.RecoverReviewError{Reason: "retained_task_tip_not_included", Noun: "feature", NodeID: "F-aaaaaa", TaskID: "T-bbbbbb", Ref: "refs/heads/task", Prerequisite: "preserve later task work separately"}
	var buf bytes.Buffer
	code, err := WriteError(&buf, "sync", failure)
	require.NoError(t, err)
	require.Equal(t, 1, code)
	var envelope struct {
		OK    bool      `json:"ok"`
		Error ErrorBody `json:"error"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	require.False(t, envelope.OK)
	require.Equal(t, failure.Reason, envelope.Error.Reason)
	require.Equal(t, failure.NodeID, envelope.Error.Node)
	require.Equal(t, failure.TaskID, envelope.Error.Task)
	require.Equal(t, failure.Ref, envelope.Error.Ref)
	require.Equal(t, failure.Prerequisite, envelope.Error.Prerequisite)
}

func TestRecoverReviewCLIRecordOnlyAndHonestRetry(t *testing.T) {
	root := initGatedGitRepo(t, "true")
	gitRun(t, root, "branch", "-M", "main")
	declareIntegrationBranchCLI(t, root)
	walkFeatureToActive(t)
	// The shared CLI helper authors a fixed ID; this fixture uses real minted
	// identities, so author and approve the contract for its actual feature.
	setupTree := loadTestTree(t)
	setupTask := setupTree.Tasks[soleTaskID(t)]
	evalPath := filepath.Join(featureDir(t), "feature.eval.md")
	evalData, err := os.ReadFile(evalPath)
	require.NoError(t, err)
	contract, err := store.ParseEvalFile(evalData)
	require.NoError(t, err)
	contract.Node = setupTask.FeatureID
	authored, err := yaml.Marshal(contract)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(evalPath, []byte("---\n"+string(authored)+"---\n\n# Acceptance\n"), 0644))
	refreshedOut, refreshedErr, refreshedCode := execCLI(t, "approve", "feature", cliFeatureKey, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, refreshedCode, refreshedOut+refreshedErr)
	_, _, code := execCLI(t, "configure", "feature", cliFeatureKey, "--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "task", cliFeatureKey+"/transcribe-the-schema", "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)
	_, _, code = execCLI(t, "approve", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	tree := loadTestTree(t)
	task := tree.Tasks[tid]
	feature := tree.Features[task.FeatureID]
	gitRun(t, root, "branch", feature.Branch)
	checkoutTaskBranchCLI(t, tid)
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "task checked")
	_, _, code = execCLI(t, "submit", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "task submission state")
	gitRun(t, root, "checkout", feature.Branch)
	gitRun(t, root, "merge", "--no-ff", "-m", "assemble rider", task.Branch)
	reviewed, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	gitRun(t, root, "checkout", "main")
	gitRun(t, root, "merge", "--no-ff", "-m", "human review merge", feature.Branch)
	merged, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	gitRun(t, root, "remote", "add", "origin", "https://example.invalid/review.git")
	args := []string{"--json", "submit", "feature", cliFeatureKey, "--recover-merged", "--head", strings.TrimPrefix(reviewed.Value, "git:"), "--merge-commit", strings.TrimPrefix(merged.Value, "git:"), "--pr", "155", "--reason", "authorized bookkeeping", "--contributor", "jc"}
	out, _, code := execCLI(t, args...)
	require.Equal(t, 0, code, out)
	var envelope struct {
		OK   bool              `json:"ok"`
		Data RecoverReviewView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.True(t, envelope.OK)
	require.True(t, envelope.Data.Changed)
	require.True(t, envelope.Data.EvidenceValidated)
	require.Equal(t, "caller_supplied", envelope.Data.PRAssociation)
	require.NotNil(t, envelope.Data.RecoveredReview)
	require.Equal(t, "in_review", loadTestTree(t).Tasks[tid].Status)
	require.Equal(t, "active", loadTestTree(t).Features[feature.ID].Status)
	statePath := filepath.Join(root, ".tiller/epics", feature.EpicKey, "features", filepath.Base(*feature.Key), "feature.yml")
	before, err := os.ReadFile(statePath)
	require.NoError(t, err)
	out, _, code = execCLI(t, args...)
	require.Equal(t, 0, code, out)
	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.False(t, envelope.Data.Changed)
	require.False(t, envelope.Data.EvidenceValidated)
	after, err := os.ReadFile(statePath)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, store.NounFeature, envelope.Data.Noun)
}

func TestSubmitAssumptionFlagsRejectEmptyPresenceOutsideTask(t *testing.T) {
	layReadTree(t)
	for _, noun := range []string{"feature", "epic", "bug", "chore", "tweak"} {
		for _, flag := range []string{"--assume-question=", "--reason="} {
			out, _, code := execCLI(t, "--json", "submit", noun, "not-a-node", flag)
			require.Equal(t, 2, code)
			var envelope struct {
				Error ErrorBody `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &envelope))
			require.Equal(t, "bad_args", envelope.Error.Reason)
		}
	}
	out, _, code := execCLI(t, "--json", "submit", "task", "not-a-node", "--reason=")
	require.Equal(t, 2, code)
	var envelope struct {
		Error ErrorBody `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.Equal(t, "not_found", envelope.Error.Reason)
}
