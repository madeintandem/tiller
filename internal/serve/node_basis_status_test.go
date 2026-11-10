package serve_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// setFeatureStatusOnDisk rewrites the fixture feature's status in its
// feature.yml, leaving accepted_against where approval recorded it. That is
// the state a done feature is in after an approved correct-eval amendment
// edits its eval: the pin still names the old blob, the file no longer
// matches it, and the node is past every verb the pin guards.
func setFeatureStatusOnDisk(t *testing.T, root, status string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "schema-engine", "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Status = status
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// TestNodePageBasisFollowsTheDriftSweepsStatusRule pins the node page to the
// drift report's own rule for which nodes can carry an active_basis finding
// (drift.ActiveBasisApplies). A ready feature whose eval changed after
// approval reports the drift and its refresh-basis remedy; the same feature at
// done reports nothing, because `tiller drift` does not list it and the CLI
// refuses `approve --refresh-basis` from done.
func TestNodePageBasisFollowsTheDriftSweepsStatusRule(t *testing.T) {
	root := approvedTaskRoot(t)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	type basisPayload struct {
		Drifted []string `json:"drifted"`
		Command string   `json:"command"`
	}
	read := func() *basisPayload {
		t.Helper()
		var got struct {
			ActiveBasis *basisPayload `json:"active_basis"`
		}
		require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/schema-engine", &got))
		return got.ActiveBasis
	}

	writeUnder(t, root, ".tiller/epics/form-engine/features/schema-engine/feature.eval.md",
		validFeatureEval+"\nCorrected after approval.\n")

	got := read()
	require.NotNil(t, got, "a ready feature with a changed eval reports its basis drift")
	require.Contains(t, got.Drifted, ".tiller/epics/form-engine/features/schema-engine/feature.eval.md")
	require.Equal(t, "tiller approve feature form-engine/schema-engine --refresh-basis", got.Command)

	setFeatureStatusOnDisk(t, root, "done")
	require.Nil(t, read(), "a done feature carries no basis finding and no remedy the CLI refuses")
}
