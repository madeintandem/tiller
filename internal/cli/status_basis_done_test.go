package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestStatusOffersNoBasisRemedyTheCLIRefuses pins status to the drift sweep's
// status rule (drift.ActiveBasisApplies): a ready feature whose eval changed
// after approval prints its basis drift and the refresh-basis remedy; the same
// feature at done prints neither, because `tiller approve --refresh-basis`
// refuses from done and `tiller drift` does not list it.
func TestStatusOffersNoBasisRemedyTheCLIRefuses(t *testing.T) {
	dir := layApprovedFeatureWithTask(t)
	featDir := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading")

	evalPath := filepath.Join(featDir, "feature.eval.md")
	data, err := os.ReadFile(evalPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(evalPath, append(data, []byte("\nCorrected after approval.\n")...), 0o644))

	out, _, code := execCLI(t, "status", "feature", cliFeatureKey)
	require.Equal(t, 0, code)
	require.Contains(t, out, "active_basis: feature.eval.md")
	require.Contains(t, out, "--refresh-basis")

	ymlPath := filepath.Join(featDir, "feature.yml")
	raw, err := os.ReadFile(ymlPath)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	doc.Status = "done"
	raw, err = yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(ymlPath, raw, 0o644))

	out, _, code = execCLI(t, "status", "feature", cliFeatureKey)
	require.Equal(t, 0, code)
	require.NotContains(t, out, "active_basis")
	require.NotContains(t, out, "--refresh-basis")
}
