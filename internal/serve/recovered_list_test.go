package serve_test

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// recoveredFeatureFixture is nodeFixture with its one feature done and carrying
// a recovered review, current acceptance, and policy warnings.
func recoveredFeatureFixture(t *testing.T) (string, *store.Feature, func(string, any), string) {
	t.Helper()
	root := nodeFixture(t)
	tree, err := store.Load(root)
	require.NoError(t, err)
	var feature *store.Feature
	for _, candidate := range tree.Features {
		if candidate.Key != nil && *candidate.Key == "form-engine/schema-engine" {
			feature = candidate
			break
		}
	}
	require.NotNil(t, feature)
	tasks := tree.FeatureTasks[feature.ID]
	require.Len(t, tasks, 1)
	task := tasks[0]
	featurePath := ".tiller/epics/form-engine/features/schema-engine/feature.yml"
	taskPath := ".tiller/epics/form-engine/features/schema-engine/tasks/parse-the-schema/task.yml"
	commit := store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + strings.Repeat("a", 40)}
	blob := func(path, oid string) store.RevisionRef {
		return store.RevisionRef{Kind: store.KindGitBlob, Value: "git:" + strings.Repeat(oid, 40), Path: path}
	}
	feature.Status, feature.ReviewGrain, feature.Branch = "done", store.ReviewGrainFeature, "historical-review"
	feature.ReviewRef = &store.ReviewRef{Branch: feature.Branch, Head: commit}
	feature.RecoveredReview = &store.RecoveredReview{Branch: feature.Branch, Head: commit, MergeCommit: commit, IntegrationBranch: "main", Basis: "contained", RecoveredBy: "jc", RecoveredAt: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC), Reason: "Authorized recovery",
		Tasks:           []store.RecoveredTask{{ID: task.ID, Path: taskPath, State: blob(taskPath, "d")}},
		PolicySnapshots: []store.HistoricalPolicySnapshot{{Path: ".tiller/tiller.yml", Source: blob(".tiller/tiller.yml", "b")}, {NodeID: feature.ID, Path: featurePath, Source: blob(featurePath, "c"), Observations: []store.HistoricalPolicyObservation{{Gate: "evals", Status: "pending", UnresolvedCriteria: []string{"C8"}}}}, {NodeID: task.ID, Path: taskPath, Source: blob(taskPath, "d")}}}
	feature.Gates = map[string]store.Gate{"evals": {Status: "pass", Criteria: map[string]store.Criterion{"C1": {Status: "pass"}}}}
	feature.PolicyViolations = []store.PolicyViolation{
		{Gate: "evals", Status: "pending", Observed: commit, SourceRevision: blob(featurePath, "c"), UnresolvedCriteria: []string{"C8"}},
		{Gate: "evals", Status: "fail", Observed: commit},
	}
	task.Status, task.Started, task.Owner = "done", true, "jc"
	tree.Manifest.Git.IntegrationBranch = "main"
	tree.Manifest.Gates.Required = map[string]store.GateSpec{"evals": {Kind: store.GateKindCommand, On: store.StringList{store.NounFeature}}}
	save := func(path string, value any) {
		data, err := yaml.Marshal(value)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, path), data, 0o644))
	}
	save(".tiller/tiller.yml", tree.Manifest)
	save(featurePath, feature)
	save(taskPath, task)
	contract := fmt.Sprintf("---\nnode: %s\ncriteria:\n  - id: C1\n    statement: Current approved behavior\n    required: true\n    binding: {type: manual}\n---\n", feature.ID)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/form-engine/features/schema-engine/feature.eval.md"), []byte(contract), 0o644))
	_, err = store.Load(root)
	require.NoError(t, err)
	return root, feature, save, featurePath
}

func TestRecoveredCompletedListClassifiesCurrentAcceptanceIndependently(t *testing.T) {
	root, feature, save, featurePath := recoveredFeatureFixture(t)
	srv := httptest.NewServer(serve.ListHandler(root))
	t.Cleanup(srv.Close)
	read := func() views.ListItem {
		var result struct {
			Nodes []views.ListItem `json:"nodes"`
		}
		require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=features&status=done", &result))
		require.Len(t, result.Nodes, 1)
		return result.Nodes[0]
	}
	row := read()
	require.NotNil(t, row.CurrentAcceptance)
	require.True(t, row.CurrentAcceptance.AcceptanceReady)
	require.Equal(t, 1, *row.HistoricalViolations)
	require.Equal(t, 1, *row.CrossingViolations)
	require.Equal(t, "valid", row.CurrentAcceptance.ContractStatus)
	require.Equal(t, "pass", row.Gates["evals"].Status)
	require.Equal(t, "pending", row.RecoveredReview.PolicySnapshots[1].Observations[0].Status)
	require.Equal(t, []string{"C8"}, row.RecoveredReview.PolicySnapshots[1].Observations[0].UnresolvedCriteria)
	feature.Gates["evals"] = store.Gate{Status: "pending", Criteria: map[string]store.Criterion{"C1": {Status: "needs_attestation"}}}
	save(featurePath, feature)
	row = read()
	require.NotNil(t, row.CurrentAcceptance)
	require.False(t, row.CurrentAcceptance.AcceptanceReady)
	require.NotEmpty(t, row.CurrentAcceptance.Outstanding)
	require.Equal(t, "pending", row.RecoveredReview.PolicySnapshots[1].Observations[0].Status)
}

// The recovered review's PR address derives from the origin the way a node's
// pr_url does: a GitHub origin yields it, any other shape omits it.
func TestRecoveredReviewCarriesThePRURLFromAGitHubOrigin(t *testing.T) {
	root, feature, save, featurePath := recoveredFeatureFixture(t)
	pr := 155
	feature.PR, feature.ReviewRef = &pr, nil
	save(featurePath, feature)
	gitRun(t, root, "init", "-q", "-b", "main")
	gitRun(t, root, "config", "user.name", "Smoke")
	gitRun(t, root, "config", "user.email", "smoke@example.test")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-qm", "fixture")
	srv := httptest.NewServer(serve.ListHandler(root))
	t.Cleanup(srv.Close)
	nodeSrv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(nodeSrv.Close)
	for _, c := range []struct{ name, origin, want string }{
		{"github", "https://github.com/madeintandem/tiller.git", "https://github.com/madeintandem/tiller/pull/155"},
		{"gitlab", "https://gitlab.com/madeintandem/tiller.git", ""},
		{"no origin", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			setOrigin(t, root, c.origin)
			var list struct {
				Nodes []struct {
					Recovered *struct {
						PR    *int   `json:"pr"`
						PRURL string `json:"pr_url"`
					} `json:"recovered_review"`
				} `json:"nodes"`
			}
			require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=features&status=done", &list))
			require.Len(t, list.Nodes, 1)
			require.NotNil(t, list.Nodes[0].Recovered)
			require.Equal(t, c.want, list.Nodes[0].Recovered.PRURL)
			var page struct {
				Node struct {
					Recovered *struct {
						PRURL string `json:"pr_url"`
					} `json:"recovered_review"`
				} `json:"node"`
			}
			require.Equal(t, 200, getJSON(t, nodeSrv, "/api/node/feature/"+listFeatureID(t, srv), &page))
			require.NotNil(t, page.Node.Recovered)
			require.Equal(t, c.want, page.Node.Recovered.PRURL)
		})
	}
}

func listFeatureID(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	var list struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=features&status=done", &list))
	return list.Nodes[0].ID
}
