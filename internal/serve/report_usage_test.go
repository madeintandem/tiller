package serve_test

// These tests pin GET /api/report/usage: the views.UsageReportOf payload over
// the committed .tiller/usage/ files, served through the full serve.Handler,
// read-only.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/usage"
	"github.com/stretchr/testify/require"
)

func writeUsageFile(t *testing.T, root string, f usage.File) {
	t.Helper()
	data, err := usage.Encode(f)
	require.NoError(t, err)
	p := filepath.Join(root, usage.Path(f.Harness, f.Session))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, data, 0o644))
}

// turnOnCapture appends usage.capture: true to the fixture's manifest.
func turnOnCapture(t *testing.T, root string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(root, ".tiller", "tiller.yml"), os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString("usage:\n  capture: true\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestUsageReportHandlerServesJSON(t *testing.T) {
	root, _ := homeFixture(t)
	turnOnCapture(t, root)
	hour := time.Now().UTC().Truncate(time.Hour)
	writeUsageFile(t, root, usage.File{
		Harness: "claude-code", Session: "s1", Reader: usage.ReaderVersion,
		Rows: []usage.Row{{Hour: hour, Stage: usage.Implementation, Model: "m", Calls: 3, Tokens: usage.Tokens{Input: 5, CacheRead: 70}}},
	})
	srv := homeRecapServer(t, root)

	var body map[string]any
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/report/usage", &body))
	require.GreaterOrEqual(t, len(body["weeks"].([]any)), 4)
	require.Equal(t, float64(75), body["kpis"].(map[string]any)["total"])
	require.Equal(t, "Claude Code", body["harnesses"].([]any)[0].(map[string]any)["label"])

	// The Project report's payload is untouched by the new route.
	var project map[string]any
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/report", &project))
	require.NotContains(t, project, "harnesses")
	require.Equal(t, project["weeks"], body["weeks"])
	require.Equal(t, true, project["agent_use"])
}

// TestUsageReportHandlerHidesUsageWithCaptureOff pins that turning capture off
// retires the tab: usage files left from before are not read or served, and
// the Project payload stops offering the tab.
func TestUsageReportHandlerHidesUsageWithCaptureOff(t *testing.T) {
	root, _ := homeFixture(t)
	writeUsageFile(t, root, usage.File{
		Harness: "claude-code", Session: "s1", Reader: usage.ReaderVersion,
		Rows: []usage.Row{{Hour: time.Now().UTC().Truncate(time.Hour), Stage: usage.Implementation, Model: "m", Calls: 1, Tokens: usage.Tokens{Input: 5}}},
	})
	bad := filepath.Join(root, usage.Path("pi", "broken"))
	require.NoError(t, os.MkdirAll(filepath.Dir(bad), 0o755))
	require.NoError(t, os.WriteFile(bad, []byte("rows: [unclosed"), 0o644))
	srv := homeRecapServer(t, root)

	var body map[string]any
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/report/usage", &body))
	require.Empty(t, body["harnesses"])
	require.Equal(t, float64(0), body["kpis"].(map[string]any)["total"])

	var project map[string]any
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/report", &project))
	require.Equal(t, false, project["agent_use"])
}

// TestUsageReportHandlerFailsOnAMalformedFile pins that a broken usage file is
// a loud 500 naming its path, as a broken event stream is for /api/report.
func TestUsageReportHandlerFailsOnAMalformedFile(t *testing.T) {
	root, _ := homeFixture(t)
	turnOnCapture(t, root)
	bad := filepath.Join(root, usage.Path("pi", "broken"))
	require.NoError(t, os.MkdirAll(filepath.Dir(bad), 0o755))
	require.NoError(t, os.WriteFile(bad, []byte("rows: [unclosed"), 0o644))
	srv := homeRecapServer(t, root)

	resp, err := srv.Client().Get(srv.URL + "/api/report/usage")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 500, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	require.Contains(t, string(raw), ".tiller/usage/pi/broken.yml")
}

func TestUsageReportHandlerRefusesWrites(t *testing.T) {
	root, _ := homeFixture(t)
	srv := homeRecapServer(t, root)

	resp, err := srv.Client().Post(srv.URL+"/api/report/usage", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
}
