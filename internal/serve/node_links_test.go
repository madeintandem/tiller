package serve_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// setFeatureReviewFacts records a branch and a verified_against commit on one
// feature by round-trip through the store type (setTaskPR's own precedent), so
// store.Load reads back a feature the handler can derive links from.
func setFeatureReviewFacts(t *testing.T, root, featureKey, branch, commit string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", "form-engine", "features", featureKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var f store.Feature
	require.NoError(t, yaml.Unmarshal(data, &f))
	f.Branch = branch
	f.VerifiedAgainst = store.VerifiedAgainst{
		Spec:   store.RevisionRef{Kind: "git_blob", Value: "git:bedf333ebd0e955a347dd9b82bb1d7fc4bd14278"},
		Commit: store.RevisionRef{Kind: "git_commit", Value: commit},
	}
	out, err := yaml.Marshal(&f)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// setTaskBasisCommit records an accepted_against pin with its approval commit
// on one task, the same round-trip.
func setTaskBasisCommit(t *testing.T, root, featureKey, taskKey, commit string) {
	t.Helper()
	path := filepath.Join(root, store.TaskRelDir("form-engine", featureKey, taskKey), store.TaskStateFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var tk store.Task
	require.NoError(t, yaml.Unmarshal(data, &tk))
	tk.AcceptedAgainst = &store.BasisRefs{
		Spec:   store.RevisionRef{Kind: "git_blob", Value: "git:4bf7c96ac7346dba2c360308c4f37ff50dbdf227"},
		Commit: store.RevisionRef{Kind: "git_commit", Value: commit},
	}
	out, err := yaml.Marshal(&tk)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

type basisLinks struct {
	Commit    string `json:"commit"`
	CommitURL string `json:"commit_url"`
}

// TestNodePageGitHubLinks pins the node header pass's two new serve-filled
// links beside pr_url (2026-08-30): a GitHub origin turns a feature's branch
// into its tree page and a recorded basis commit (accepted_against on a task,
// verified_against on a feature) into its commit page, the "git:" prefix
// stripped. The same origin rule pr_url follows applies: a non-GitHub origin
// or no origin at all omits every link (absent, never null) while the facts
// themselves still render.
func TestNodePageGitHubLinks(t *testing.T) {
	root := nodeFixture(t)
	setFeatureReviewFacts(t, root, "schema-engine", "tiller/form-engine/schema-engine", "git:828a8b0")
	setTaskBasisCommit(t, root, "schema-engine", "parse-the-schema", "git:7458e30")
	gitRun(t, root, "init")

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)
	const featurePath = "/api/node/feature/form-engine/schema-engine"
	const taskPath = "/api/node/task/T-000001"
	const repo = "https://github.com/madeintandem/tiller"

	t.Run("github origin links the branch and both basis commits", func(t *testing.T) {
		setOrigin(t, root, "git@github.com:madeintandem/tiller.git")

		status, top := rawPage(t, srv, featurePath)
		require.Equal(t, 200, status)
		var feature struct {
			Branch    string     `json:"branch"`
			BranchURL string     `json:"branch_url"`
			Verified  basisLinks `json:"verified_against"`
		}
		require.NoError(t, json.Unmarshal(top["node"], &feature))
		require.Equal(t, "tiller/form-engine/schema-engine", feature.Branch)
		require.Equal(t, repo+"/tree/tiller/form-engine/schema-engine", feature.BranchURL)
		require.Equal(t, "git:828a8b0", feature.Verified.Commit)
		require.Equal(t, repo+"/commit/828a8b0", feature.Verified.CommitURL)

		status, top = rawPage(t, srv, taskPath)
		require.Equal(t, 200, status)
		var task struct {
			Accepted basisLinks `json:"accepted_against"`
		}
		require.NoError(t, json.Unmarshal(top["node"], &task))
		require.Equal(t, "git:7458e30", task.Accepted.Commit)
		require.Equal(t, repo+"/commit/7458e30", task.Accepted.CommitURL)
	})

	for _, c := range []struct{ name, origin string }{
		{"gitlab origin", "https://gitlab.com/madeintandem/tiller.git"},
		{"no origin", ""},
	} {
		t.Run(c.name+" omits every link but keeps the facts", func(t *testing.T) {
			setOrigin(t, root, c.origin)

			status, top := rawPage(t, srv, featurePath)
			require.Equal(t, 200, status)
			var feature map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(top["node"], &feature))
			require.Contains(t, feature, "branch")
			require.NotContains(t, feature, "branch_url")
			var verified map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(feature["verified_against"], &verified))
			require.Contains(t, verified, "commit")
			require.NotContains(t, verified, "commit_url")

			status, top = rawPage(t, srv, taskPath)
			require.Equal(t, 200, status)
			var task map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(top["node"], &task))
			var accepted map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(task["accepted_against"], &accepted))
			require.Contains(t, accepted, "commit")
			require.NotContains(t, accepted, "commit_url")
		})
	}
}
